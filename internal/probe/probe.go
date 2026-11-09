// Package probe 对直播流地址做连通性/首字节延迟/吞吐测量，并按可达性排序，
// 供失败自动切换挑选最优备用源。
package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// probeTimeout 是单条 URL 的探测超时。
const probeTimeout = time.Second

// sampleBytes 是吞吐采样读取量：读到这么多字节就停止，足够区分快慢源。
const sampleBytes = 128 * 1024

// maxRankConcurrency 是 Rank 并行探测的上限：备用源列表通常不到 10 条，
// 8 路并发足够，太多并发只会给源站造成无意义的突发连接。
const maxRankConcurrency = 8

// Result 是一次测速的结果。
type Result struct {
	URL       string
	Reachable bool
	Latency   time.Duration // 首字节延迟
	Speed     int64         // 字节/秒（估算）
	Err       error
}

// Prober 对 URL 做测速与排序。
type Prober struct {
	Client *http.Client
}

// NewProber 返回默认 Prober（单 URL 超时 1s）。
func NewProber() *Prober {
	return &Prober{Client: &http.Client{Timeout: probeTimeout}}
}

// Probe 测量单个 URL：GET 请求，读至多 sampleBytes 字节，统计首字节延迟与吞吐。
// 不读完整响应体（直播流是无限流，读到采样量即止）。
func (p *Prober) Probe(ctx context.Context, url string, headers map[string]string) Result {
	r := Result{URL: url}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		r.Err = err
		return r
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := p.Client.Do(req)
	if err != nil {
		r.Err = err
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		return r
	}
	r.Reachable = true
	r.Latency = time.Since(start)
	n, _ := io.CopyN(io.Discard, resp.Body, sampleBytes)
	elapsed := time.Since(start)
	if elapsed > 0 && n > 0 {
		r.Speed = int64(float64(n) / elapsed.Seconds())
	}
	return r
}

// Rank 对 URL 列表测速并排序：可达优先，其次吞吐降序，再次延迟升序。
// 单条 URL 直接返回（不探测）。测速失败的源排到末尾（保留原始相对顺序）。
// 各 URL 并行探测（单条仍有 1s 超时兜底）：串行时 N 个备用源要 N 秒，
// failover.Load 每次播放都等 Rank 结果；并行后耗时接近最慢的一条。
func (p *Prober) Rank(ctx context.Context, urls []string, headers map[string]string) []string {
	if len(urls) <= 1 {
		return append([]string(nil), urls...)
	}
	type scored struct {
		url string
		res Result
	}
	items := make([]scored, len(urls))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxRankConcurrency)
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items[i] = scored{url: u, res: p.Probe(ctx, u, headers)}
		}(i, u)
	}
	wg.Wait()
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := items[i].res, items[j].res
		if ri.Reachable != rj.Reachable {
			return ri.Reachable
		}
		if ri.Speed != rj.Speed {
			return ri.Speed > rj.Speed
		}
		return ri.Latency < rj.Latency
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.url
	}
	return out
}
