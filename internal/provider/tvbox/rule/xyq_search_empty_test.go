package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 搜索列表里标题为空的项应被跳过（parseHTMLItems 的注释写明“跳过”，
// 但分支体是空的、漏了 continue，导致空标题项仍被收进结果）。
func TestXYQSearchSkipsEmptyTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<div class="module-items">
			<div class="module-search-item"><h3><a href="/v/1">有标题</a></h3></div>
			<div class="module-search-item"><h3><a href="/v/2"></a></h3></div>
		</div>`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:      srv.URL,
		SearchURL:    srv.URL + "/vodsearch/{wd}",
		SearchArray:  ".module-items&&.module-search-item",
		SearchTitle:  "h3&&Text",
		SearchLink:   "h3&&a&&href",
		SearchPrefix: srv.URL,
	}, NewClient(nil))

	items, err := e.Search(context.Background(), "测试")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("搜索 = %d 项，期望 1（空标题项应跳过），实际 %#v", len(items), items)
	}
	if items[0].Title != "有标题" {
		t.Fatalf("首项标题 = %q，期望 有标题", items[0].Title)
	}
}
