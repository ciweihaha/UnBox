package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

func TestNewJarRejectsUnknownClass(t *testing.T) {
	if _, err := NewJar(config.Site{Key: "x", API: "csp_Unknown", Ext: "https://x/r.json"}); err == nil {
		t.Fatal("未知 csp 类应返回错误")
	}
}

func TestNewJarAcceptsXBPQ(t *testing.T) {
	p, err := NewJar(config.Site{Key: "x", API: "csp_XBPQ", Ext: "https://x/r.json"})
	if err != nil || p == nil {
		t.Fatalf("NewJar = %v, %v", p, err)
	}
}

// TestJarLoadFailureRetries 是回归测试（第 38 轮）：规则加载失败不能
// 被缓存——第 34 轮修了 Spider 的同款问题，JarProvider 漏了；一次
// 瞬时 500 会让该站点在本会话内永久不可用，下次调用应重新加载。
func TestJarLoadFailureRetries(t *testing.T) {
	var calls atomic.Int32
	ruleJSON := `{
		"主页url":"http://example.com/",
		"分类":"电影$1",
		"分类url":"http://example.com/c/{cateId}/{catePg}",
		"数组":"class=\"item\"&&</li>",
		"标题":"title=\"&&\"",
		"链接":"href=\"&&\""
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "瞬时故障", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(ruleJSON))
	}))
	defer srv.Close()

	p, err := NewJar(config.Site{Key: "x", API: "csp_XBPQ", Ext: srv.URL + "/rule.json"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Home(context.Background()); err == nil {
		t.Fatal("第一次加载 500 应报错")
	}
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 1 || secs[0].ID != "1" {
		t.Fatalf("重试后 Home 应成功: %+v err=%v", secs, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("应恰好加载两次，实际 %d 次", calls.Load())
	}
}

func TestJarProviderEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rule.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"主页url":"` + "http://" + r.Host + `/",
			"分类":"电影$1",
			"分类url":"http://` + r.Host + `/c/{cateId}/{catePg}",
			"数组":"class=\"item\"&&</li>",
			"标题":"title=\"&&\"",
			"链接":"href=\"&&\"",
			"播放数组":"class=\"play\">&&</div>",
			"播放列表":"<a&&</a>",
			"播放标题":"<span>&&</span>",
			"播放链接":"href=\"&&\""
		}`))
	})
	mux.HandleFunc("/c/1/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<li class="item"><a title="甲" href="/v/1"></a></li>`))
	})
	mux.HandleFunc("/v/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>甲</title><div class="play"><a href="/p/1"><span>第1集</span></a></div>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewJar(config.Site{Key: "x", API: "csp_XBPQ", Ext: srv.URL + "/rule.json"})
	if err != nil {
		t.Fatal(err)
	}
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 1 || secs[0].ID != "1" {
		t.Fatalf("Home = %#v, %v", secs, err)
	}
	page, err := p.Browse(context.Background(), "1", 1)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("Browse = %#v, %v", page, err)
	}
	media, err := p.Detail(context.Background(), page.Items[0].ID)
	if err != nil || len(media.Episodes) != 1 {
		t.Fatalf("Detail = %#v, %v", media, err)
	}
	stream, err := p.Resolve(context.Background(), media.Episodes[0].ID)
	if err != nil || stream.URL != srv.URL+"/p/1" {
		t.Fatalf("Resolve = %#v, %v", stream, err)
	}
}
