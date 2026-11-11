package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveCapsFanOut 是回归测试（第 24 轮）：单个索引含数千条目时，
// 展开不得为每条都起一个 goroutine（无界扇出会把内存与连接打爆）；
// 超过节点上限的部分应被跳过并报错，但已展开的终端配置仍要返回。
func TestResolveCapsFanOut(t *testing.T) {
	const entries = 3000
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString(`{"urls":[`)
		for i := 0; i < entries; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"url":"%s/leaf%d","name":"线路%d"}`, srv.URL, i, i)
		}
		b.WriteString(`]}`)
		_, _ = w.Write([]byte(b.String()))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	done := make(chan struct{})
	var cfgs []*Config
	var err error
	go func() {
		cfgs, err = r.Resolve(context.Background(), srv.URL+"/index")
		close(done)
	}()
	select {
	case <-done:
	case <-timeAfter():
		t.Fatal("展开 3000 条目超时（疑似无界 goroutine 扇出拖垮）")
	}
	if err == nil || !strings.Contains(err.Error(), "超过上限") {
		t.Fatalf("应报节点数超过上限，实际 err: %v", err)
	}
	if len(cfgs) == 0 {
		t.Fatal("超限时已展开的终端配置仍应返回，实际 0 个")
	}
	if len(cfgs) > maxResolveNodes {
		t.Fatalf("展开了 %d 个配置，超过节点上限 %d", len(cfgs), maxResolveNodes)
	}
}
