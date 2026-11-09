package tvbox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/provider"
)

// 回归测试（第 14 轮）：同一 Spider 并发调用时，各自执行读到的 ctx 必须是自己的。
// SetContext 若在 Spider.mu 锁外调用，并发的另一次调用会改掉 Engine.ctx，
// 让执行中的 req() 读到别人的 ctx——别人结束时一 cancel，本次请求就跟着失败
// （或反过来丢掉自己的超时）。修法：SetContext 只在持 Spider.mu 期间调用。
func TestSpiderConcurrentSearchKeepsOwnContext(t *testing.T) {
	slowEntered := make(chan struct{})
	releaseSlow := make(chan struct{})
	var slowOnce sync.Once
	var script string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/spider.js":
			_, _ = w.Write([]byte(script))
		case "/slow":
			slowOnce.Do(func() { close(slowEntered) })
			<-releaseSlow
			_, _ = w.Write([]byte("slow"))
		case "/fast":
			_, _ = w.Write([]byte("fast"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	script = fmt.Sprintf(`
function search(wd){
  var slow = req(%q);
  var fast = req(%q);
  return [{vod_id:"1", vod_name: slow.content + "+" + fast.content}];
}
`, srv.URL+"/slow", srv.URL+"/fast")

	p, err := NewSpider(config.Site{Key: "ctx", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		items []provider.Item
		err   error
	}
	// A 用一直有效的 ctx 搜索；脚本在 /slow 处阻塞，A 此时正持 Spider.mu 执行 JS。
	aDone := make(chan result, 1)
	go func() {
		items, err := p.Search(context.Background(), "q")
		aDone <- result{items, err}
	}()
	select {
	case <-slowEntered:
	case <-time.After(10 * time.Second):
		t.Fatal("搜索未进入 /slow，脚本未按预期执行")
	}

	// B 用已取消的 ctx 搜索：修好前它的 SetContext 在锁外立刻执行，
	// 再在 ensureLoaded 的 mu 上阻塞；修好后它要等 A 释放 mu 才能绑定 ctx。
	ctxB, cancelB := context.WithCancel(context.Background())
	cancelB()
	bDone := make(chan result, 1)
	go func() {
		items, err := p.Search(ctxB, "q")
		bDone <- result{items, err}
	}()
	// 给 B 留出执行 SetContext 的时间（第一条语句即 SetContext，300ms 绰绰有余）。
	time.Sleep(300 * time.Millisecond)
	close(releaseSlow)

	res := <-aDone
	if res.err != nil {
		t.Fatalf("A 的搜索应用自己的 ctx 成功，实际 err=%v（执行中被 B 的已取消 ctx 串味）", res.err)
	}
	if len(res.items) != 1 || res.items[0].Title != "slow+fast" {
		t.Fatalf("A 的搜索结果异常: %+v", res.items)
	}
	// B 自己的 ctx 已取消，B 失败才是正确行为；只等它收尾，不让 goroutine 泄漏。
	select {
	case bRes := <-bDone:
		if bRes.err == nil {
			t.Fatalf("B 用已取消的 ctx 搜索应失败，实际 items=%+v", bRes.items)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("B 的搜索未返回")
	}
}

// 回归测试（第 14 轮）：Resolve 必须绑定本次调用的 ctx，不能沿用上次残留的 ctx。
// 此前 Resolve 完全不调 SetContext：VodPlay 里 JS 的 req() 用的是上次调用绑的
// ctx，若那次是已取消的全站搜索 siteCtx，解析请求会立刻失败；Resolve 还会把
// VodPlay 的错误吞掉、静默退回未解析的剧集地址。
func TestSpiderResolveBindsOwnContext(t *testing.T) {
	var script string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/spider.js":
			_, _ = w.Write([]byte(script))
		case "/play":
			_, _ = w.Write([]byte("https://example.com/stream.m3u8"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	script = fmt.Sprintf(`
function detail(id){ return {vod_id:id, vod_name:"甲", vod_pic:"", vod_content:"", vod_year:"2026", vod_area:"", vod_play_from:"线路一", vod_play_url:"第01集$https://example.com/ep1"} }
function playerContent(flag, id, vipFlags){ var r = req(%q); return JSON.stringify({parse:0, url: r.content}); }
`, srv.URL+"/play")

	p, err := NewSpider(config.Site{Key: "ctx2", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}
	// 先用一个 ctx 做详情（填充剧集缓存），随后取消它：
	// 此前绑在 Engine 上的就是这个已取消的 ctx。
	ctxOld, cancelOld := context.WithCancel(context.Background())
	media, err := p.Detail(ctxOld, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Episodes) != 1 {
		t.Fatalf("详情应返回 1 集: %+v", media)
	}
	cancelOld()
	// 再用全新有效的 ctx 解析：必须走本次 ctx 请求到真实流地址，
	// 而不是因旧 ctx 已取消、VodPlay 失败后退回未解析的剧集地址。
	stream, err := p.Resolve(context.Background(), media.Episodes[0].ID)
	if err != nil {
		t.Fatalf("Resolve err=%v", err)
	}
	if stream.URL != "https://example.com/stream.m3u8" {
		t.Fatalf("Resolve URL=%q，应为解析出的流地址（沿用了已取消的旧 ctx，导致 VodPlay 失败后退回未解析地址）", stream.URL)
	}
}
