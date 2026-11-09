package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
