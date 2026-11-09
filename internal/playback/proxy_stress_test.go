package playback

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// TestProxyConcurrentRegisterFetchRelease 是播放代理的压力测试：模拟播放器
// 与 WebView 的真实并发——多个 goroutine 同时 Register 新流、经代理拉取
// HLS 清单与分片、再 Release 释放。streams 注册表由 RWMutex 保护，
// serveHTTP 在每个请求 goroutine 里读表，Register/Release 在调用方
// goroutine 里写表；本测试在 -race 下跑，任何漏锁的读写都会被检出。
//
// 规模：16 goroutine × 32 轮 = 512 次 Register + 512 次清单拉取 +
// 512 次分片拉取 + 512 次 Release，全部走本地 httptest，不依赖外网。
func TestProxyConcurrentRegisterFetchRelease(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live/index.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\nsegment.ts\n")
		case "/live/segment.ts":
			_, _ = io.WriteString(w, "seg-body")
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.Client(), time.Hour)
	t.Cleanup(func() { _ = proxy.Close() })
	client := &http.Client{Timeout: 10 * time.Second}

	const goroutines = 16
	const rounds = 32

	var failed atomic.Int64
	errs := make(chan string, goroutines*rounds*4)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				proxyURL, err := proxy.Register(context.Background(), player.Stream{
					URL:  upstream.URL + "/live/index.m3u8",
					Kind: player.StreamHLS,
					Headers: map[string]string{
						"Referer": "https://origin.example/",
					},
				})
				if err != nil {
					errs <- fmt.Sprintf("worker %d round %d: Register: %v", worker, i, err)
					failed.Add(1)
					continue
				}
				// 拉取清单：必须 200，且分片地址已被重写为本地代理地址。
				body, status, err := fetch(t, client, proxyURL)
				if err != nil || status != http.StatusOK {
					errs <- fmt.Sprintf("worker %d round %d: GET 清单: status=%d err=%v", worker, i, status, err)
					failed.Add(1)
					continue
				}
				if strings.Contains(body, "\nsegment.ts\n") {
					errs <- fmt.Sprintf("worker %d round %d: 清单仍含未代理地址:\n%s", worker, i, body)
					failed.Add(1)
					continue
				}
				refs := manifestProxyURLs(body)
				if len(refs) != 1 {
					errs <- fmt.Sprintf("worker %d round %d: 重写地址数 = %d, want 1; body:\n%s", worker, i, len(refs), body)
					failed.Add(1)
					continue
				}
				// 拉取重写后的分片地址：必须 200 且内容一致。
				segBody, status, err := fetch(t, client, refs[0])
				if err != nil || status != http.StatusOK || segBody != "seg-body" {
					errs <- fmt.Sprintf("worker %d round %d: GET 分片: status=%d body=%q err=%v", worker, i, status, segBody, err)
					failed.Add(1)
					continue
				}
				// 释放后 token 必须立即失效。
				if err := proxy.Release(proxyURL); err != nil {
					errs <- fmt.Sprintf("worker %d round %d: Release: %v", worker, i, err)
					failed.Add(1)
					continue
				}
				_, status, err = fetch(t, client, proxyURL)
				if err != nil || status != http.StatusNotFound {
					errs <- fmt.Sprintf("worker %d round %d: 释放后状态 = %d, want 404 (err=%v)", worker, i, status, err)
					failed.Add(1)
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
	if n := failed.Load(); n != 0 {
		t.Fatalf("并发压力测试失败 %d 次（共 %d 轮）", n, goroutines*rounds)
	}
}

// TestProxyConcurrentSharedTokenFetch 覆盖另一种真实并发：同一条已注册的流
// 被多个播放请求同时拉取（WebView 列表页 + 播放页可能同时请求同一清单）。
// lookup 与 clientFor 在多个 serveHTTP goroutine 里并行读同一注册表项，
// 注册表项的 secret/stream 是深拷贝后返回的，调用方不得互相干扰。
func TestProxyConcurrentSharedTokenFetch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/live/index.m3u8" {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\nsegment.ts\n")
			return
		}
		_, _ = io.WriteString(w, "seg-body")
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.Client(), time.Hour)
	t.Cleanup(func() { _ = proxy.Close() })
	client := &http.Client{Timeout: 10 * time.Second}

	const tokens = 8
	const goroutines = 16
	const rounds = 64

	proxyURLs := make([]string, tokens)
	for i := 0; i < tokens; i++ {
		proxyURL, err := proxy.Register(context.Background(), player.Stream{
			URL:  upstream.URL + "/live/index.m3u8",
			Kind: player.StreamHLS,
		})
		if err != nil {
			t.Fatalf("Register token %d: %v", i, err)
		}
		proxyURLs[i] = proxyURL
	}

	var failed atomic.Int64
	errs := make(chan string, goroutines*rounds)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				proxyURL := proxyURLs[(worker+i)%tokens]
				body, status, err := fetch(t, client, proxyURL)
				if err != nil || status != http.StatusOK {
					errs <- fmt.Sprintf("worker %d round %d: GET: status=%d err=%v", worker, i, status, err)
					failed.Add(1)
					continue
				}
				if refs := manifestProxyURLs(body); len(refs) != 1 {
					errs <- fmt.Sprintf("worker %d round %d: 重写地址数 = %d, want 1", worker, i, len(refs))
					failed.Add(1)
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
	if n := failed.Load(); n != 0 {
		t.Fatalf("共享 token 并发拉取失败 %d 次（共 %d 次）", n, goroutines*rounds)
	}
}

// fetch 用给定 client 拉取 URL，返回 body 与状态码；不在 goroutine 里
// 直接调 t.Fatalf（testing.T 的 FailNow 只能在测试 goroutine 调用）。
func fetch(t *testing.T, client *http.Client, rawURL string) (string, int, error) {
	t.Helper()
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}
