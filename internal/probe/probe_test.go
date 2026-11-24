package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/httpx"
)

func TestProbeReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(make([]byte, 64*1024))
	}))
	defer srv.Close()
	p := NewProber()
	r := p.Probe(context.Background(), srv.URL, nil)
	if !r.Reachable || r.Latency <= 0 || r.Speed <= 0 || r.Err != nil {
		t.Fatalf("Probe = %+v，期望可达且测出延迟/吞吐", r)
	}
}

func TestProbeUnreachable(t *testing.T) {
	p := NewProber()
	r := p.Probe(context.Background(), "http://127.0.0.1:1/nope", nil)
	if r.Reachable || r.Err == nil {
		t.Fatalf("Probe = %+v，期望不可达", r)
	}
}

func TestRankPutsReachableFirst(t *testing.T) {
	var reachable, dead string
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer deadSrv.Close()
	reachable = deadSrv.URL
	dead = "http://127.0.0.1:1/nope"
	got := NewProber().Rank(context.Background(), []string{dead, reachable}, nil)
	if got[0] != reachable {
		t.Fatalf("Rank = %v，期望可达源排最前", got)
	}
}

func TestRankSingleURLUnchanged(t *testing.T) {
	got := NewProber().Rank(context.Background(), []string{"http://x/1"}, nil)
	if len(got) != 1 || got[0] != "http://x/1" {
		t.Fatalf("Rank = %v", got)
	}
}

// TestRankProbesConcurrently 备用源测速应并行：串行时 4 个各慢 500ms 的源
// 要 2 秒才排好序（播放加载直接等 Rank 结果），并行应接近单次延迟。
func TestRankProbesConcurrently(t *testing.T) {
	const n = 4
	const delay = 500 * time.Millisecond
	urls := make([]string, 0, n)
	var servers []*httptest.Server
	for i := 0; i < n; i++ {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(delay)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("x"))
		}))
		servers = append(servers, srv)
		urls = append(urls, srv.URL)
	}
	defer func() {
		for _, srv := range servers {
			srv.Close()
		}
	}()
	start := time.Now()
	got := NewProber().Rank(context.Background(), urls, nil)
	elapsed := time.Since(start)
	if len(got) != n {
		t.Fatalf("Rank = %v", got)
	}
	// 串行下限约 n*delay=2s；并行应接近单次 delay。阈值 1.5s 留足余量。
	if elapsed >= 1500*time.Millisecond {
		t.Fatalf("Rank 疑似串行测速，4 个 500ms 源耗时 %v", elapsed)
	}
}

func TestProbeBoundsTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer slow.Close()
	start := time.Now()
	NewProber().Probe(context.Background(), slow.URL, nil)
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("Probe 未受 1s 探测超时约束，耗时 %v", elapsed)
	}
}

// TestProbeUsesEnvProxy：测速必须走代理选择（环境变量优先，其次系统代理），
// 不能用裸 http.Client 直连。2026-11-22 笔记本实测：直连到 Cloudflare
// 只有约 20B/s，备用源测速全判死、故障切换无源可选；走本机代理 2.6 秒就通。
//
// 注意：Linux 上裸客户端的 DefaultTransport 本来就认环境变量，
// 行为测试分不出差别；真正的差别在 Windows 系统代理（只改注册表、
// 不配环境变量时，DefaultTransport 直接直连）。所以这里钉接线：
// 传输层的 Proxy 必须是统一选择函数 httpx.ProxyForRequest。
func TestProbeUsesEnvProxy(t *testing.T) {
	p := NewProber()
	tr, ok := p.Client.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("测速客户端没有显式传输层（Transport = %v），会落到 DefaultTransport 直连", p.Client.Transport)
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(httpx.ProxyForRequest).Pointer() {
		t.Fatalf("测速客户端没走统一代理选择函数 httpx.ProxyForRequest，系统代理（v2rayN）下会直连慢路")
	}

	// 行为烟囱：经环境变量代理能测到一个直连 DNS 失败的域名。
	var viaProxy atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 64*1024))
	}))
	defer proxy.Close()
	setTestProxyEnv(t, proxy.URL)

	r := p.Probe(context.Background(), "http://probe-test.invalid/x", nil)
	if !r.Reachable || r.Err != nil {
		t.Fatalf("Probe = %+v，想要经代理可达（请求没走代理时该域名 DNS 失败）", r)
	}
	if viaProxy.Load() != 1 {
		t.Fatalf("经代理的请求次数 = %d，想要 1", viaProxy.Load())
	}
}

// setTestProxyEnv 把测试进程的代理环境变量指向本地代理桩，
// 并清空 NO_PROXY（目标域名不在直连名单里才进代理）。
func setTestProxyEnv(t *testing.T, proxyURL string) {
	t.Helper()
	t.Setenv("HTTP_PROXY", proxyURL)
	t.Setenv("http_proxy", proxyURL)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
}
