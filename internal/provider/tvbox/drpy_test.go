package tvbox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/httpx"
)

// newDrpyTestServer 起一个按路径返回固定 JSON 的 drpy 服务，并记录命中请求。
func newDrpyTestServer(t *testing.T, paths map[string]string) (*httptest.Server, func() []string) {
	t.Helper()
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path+"?"+r.URL.RawQuery)
		body, ok := paths[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return srv, func() []string { return hits }
}

func TestDrpyHome(t *testing.T) {
	srv, hits := newDrpyTestServer(t, map[string]string{
		"/api/homeVod": `{"class":[{"type_id":"1","type_name":"电影"},{"type_id":"2","type_name":"剧集"}],"list":[]}`,
	})
	defer srv.Close()

	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 2 || secs[0].Title != "电影" || secs[1].ID != "2" {
		t.Fatalf("Home = %+v, %v", secs, err)
	}
	if got := hits(); len(got) != 1 || got[0] != "/api/homeVod?" {
		t.Fatalf("Home 请求 = %v", got)
	}
}

// TestDrpyClientRejectsOversizedBody 同 CMS：超大响应应直接拒绝，
// 而不是全读进内存再解析。
func TestDrpyClientRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		chunk := make([]byte, 1<<20)
		for i := 0; i < maxAPIResponseBytes/(1<<20)+1; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c := newDrpyClient(srv.URL)
	if _, err := c.homeVod(context.Background()); !errors.Is(err, httpx.ErrBodyTooLarge) {
		t.Fatalf("超大响应应被拒绝，got err=%v", err)
	}
}

func TestDrpyBrowseAndSearch(t *testing.T) {
	srv, hits := newDrpyTestServer(t, map[string]string{
		"/api/category":      `{"list":[{"vod_id":1,"vod_name":"电影A","vod_pic":"http://x/p.jpg"}]}`,
		"/api/searchContent": `{"list":[{"vod_id":2,"vod_name":"搜索结果"}]}`,
	})
	defer srv.Close()

	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	pg, err := p.Browse(context.Background(), "1", 2)
	if err != nil || len(pg.Items) != 1 || pg.Items[0].Title != "电影A" || pg.Items[0].Logo != "http://x/p.jpg" {
		t.Fatalf("Browse = %+v, %v", pg, err)
	}
	items, err := p.Search(context.Background(), "测试")
	if err != nil || len(items) != 1 || items[0].Title != "搜索结果" {
		t.Fatalf("Search = %+v, %v", items, err)
	}
	got := hits()
	if len(got) != 2 || got[0] != "/api/category?pg=2&tid=1" || got[1] != "/api/searchContent?key=%E6%B5%8B%E8%AF%95" {
		t.Fatalf("请求序列 = %v", got)
	}
}

func TestDrpyDetailAndResolve(t *testing.T) {
	srv, _ := newDrpyTestServer(t, map[string]string{
		"/api/detailContent": `{"list":[{"vod_id":10,"vod_name":"测试片","vod_play_from":"线路一","vod_play_url":"第01集$https://x/a.m3u8"}]}`,
	})
	defer srv.Close()

	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	m, err := p.Detail(context.Background(), "10")
	if err != nil || len(m.Episodes) != 1 || m.Sources[0] != "线路一" || m.Episodes[0].ID != "10/0/0" {
		t.Fatalf("Detail = %+v, %v", m, err)
	}
	st, err := p.Resolve(context.Background(), "10/0/0")
	if err != nil || st.URL != "https://x/a.m3u8" {
		t.Fatalf("Resolve = %+v, %v", st, err)
	}
	if st.Headers["Referer"] == "" {
		t.Fatalf("Resolve 应带 Referer: %+v", st.Headers)
	}
}

// TestDrpyDetailCacheIsBounded 是回归测试（第 40 轮）：第 31/39 轮给 CMS
// Provider、Spider、JarProvider 的 Detail 缓存封顶时漏了 Drpy，
// 同样 300 部 × 50 集会无界膨胀到 15000 条。
func TestDrpyDetailCacheIsBounded(t *testing.T) {
	const vods, epsPerVod = 300, 50
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("ids")
		var eps strings.Builder
		for i := 1; i <= epsPerVod; i++ {
			if i > 1 {
				eps.WriteString("#")
			}
			fmt.Fprintf(&eps, "第%02d集$http://x/%s/ep%d", i, id, i)
		}
		fmt.Fprintf(w, `{"list":[{"vod_id":%q,"vod_name":"剧","vod_play_from":"线路","vod_play_url":%q}]}`, id, eps.String())
	}))
	defer srv.Close()
	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	ctx := context.Background()
	for i := 0; i < vods; i++ {
		if _, err := p.Detail(ctx, fmt.Sprintf("vod%d", i)); err != nil {
			t.Fatalf("Detail vod%d 失败: %v", i, err)
		}
	}
	p.mu.Lock()
	cached := len(p.cache)
	p.mu.Unlock()
	if cached > maxCachedEpisodes {
		t.Fatalf("缓存剧集 %d 条，超过上限 %d", cached, maxCachedEpisodes)
	}
	st, err := p.Resolve(ctx, "vod299/0/49")
	if err != nil {
		t.Fatalf("Resolve 末尾剧集失败: %v", err)
	}
	if st.URL != "http://x/vod299/ep50" {
		t.Fatalf("Resolve 末尾剧集 URL = %q", st.URL)
	}
}
