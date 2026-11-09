package playback

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

type fakeProxy struct {
	next     int
	released []string
}

func (f *fakeProxy) Register(context.Context, player.Stream) (string, error) {
	f.next++
	return fmt.Sprintf("http://127.0.0.1/proxy/test-%d", f.next), nil
}
func (f *fakeProxy) Release(proxyURL string) error {
	f.released = append(f.released, proxyURL)
	return nil
}
func (f *fakeProxy) Close() error { return nil }

type fakePlayer struct {
	loaded []player.Stream
	played int
	seeked []float64
}

func (f *fakePlayer) Load(_ context.Context, s player.Stream) error {
	f.loaded = append(f.loaded, s)
	return nil
}
func (f *fakePlayer) Play() error                             { f.played++; return nil }
func (f *fakePlayer) Pause() error                            { return nil }
func (f *fakePlayer) Seek(sec float64) error                  { f.seeked = append(f.seeked, sec); return nil }
func (f *fakePlayer) SetVolume(int) error                     { return nil }
func (f *fakePlayer) SelectTrack(player.TrackKind, int) error { return nil }
func (f *fakePlayer) State() player.State                     { return player.State{} }
func (f *fakePlayer) Events() <-chan player.Event             { return make(chan player.Event) }
func (f *fakePlayer) Close() error                            { return nil }

func TestControllerRouting(t *testing.T) {
	mpv := &fakePlayer{}
	web := &fakeProxy{}
	c := NewController(nil, web, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }

	cases := []struct {
		name   string
		webMSE bool
		stream player.Stream
		want   Backend
	}{
		{"h264 hls web-capable", true, player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS}, BackendWeb},
		{"flv web-capable", true, player.Stream{URL: "https://x/live.flv", Kind: player.StreamFLV}, BackendWeb},
		{"mp4 web-capable", true, player.Stream{URL: "https://x/a.mp4", Kind: player.StreamMP4}, BackendWeb},
		{"rtmp", true, player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP}, BackendMPV},
		{"local", true, player.Stream{URL: "file:///x/a.mkv", Kind: player.StreamLocal}, BackendMPV},
		{"hls no-mse", false, player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS}, BackendMPV},
		{"flv no-mse", false, player.Stream{URL: "https://x/live.flv", Kind: player.StreamFLV}, BackendMPV},
		{"ts no-mse", false, player.Stream{URL: "https://x/live.ts", Kind: player.StreamTS}, BackendMPV},
		{"mp4 no-mse", false, player.Stream{URL: "https://x/a.mp4", Kind: player.StreamMP4}, BackendWeb},
		{"audio web-mse", true, player.Stream{URL: "https://cdn/x.mflac", Kind: player.StreamAudio}, BackendMPV},
		{"audio no-mse", false, player.Stream{URL: "https://cdn/x.flac", Kind: player.StreamAudio}, BackendMPV},
	}
	for _, tc := range cases {
		c.SetWebMSE(tc.webMSE)
		mpv.loaded = nil
		mpv.played = 0
		got, err := c.Prepare(context.Background(), tc.stream)
		if err != nil || got.Backend != tc.want {
			t.Errorf("%s: Prepare = %+v, %v; want backend %s", tc.name, got, err, tc.want)
		}
	}
}

func TestControllerHEVCHLSRoutesToMPV(t *testing.T) {
	mpv := &fakePlayer{}
	c := NewController(nil, &fakeProxy{}, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "hvc1.1.6.L93.B0", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.Backend != BackendMPV {
		t.Fatalf("HEVC 应路由到 mpv: plan=%+v err=%v", plan, err)
	}
	if len(mpv.loaded) != 1 || mpv.played != 1 {
		t.Fatalf("mpv 后端未真正加载: loaded=%d played=%d", len(mpv.loaded), mpv.played)
	}
}

// TestControllerVideoContainerRoutesToMPV 浏览器播不了的视频容器
// （mkv/avi 等）应直接走 mpv，而不是按 HLS 兜底走 Web 再失败。
func TestControllerVideoContainerRoutesToMPV(t *testing.T) {
	mpv := &fakePlayer{}
	c := NewController(nil, &fakeProxy{}, mpv)
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://cdn.example.com/a.mkv", Kind: player.StreamVideo})
	if err != nil || plan.Backend != BackendMPV {
		t.Fatalf("mkv 应路由到 mpv: plan=%+v err=%v", plan, err)
	}
	if len(mpv.loaded) != 1 || mpv.played != 1 {
		t.Fatalf("mpv 后端未真正加载: loaded=%d played=%d", len(mpv.loaded), mpv.played)
	}
}

func TestControllerMPVBackendLoadsStream(t *testing.T) {
	mpv := &fakePlayer{}
	c := NewController(nil, &fakeProxy{}, mpv)
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP})
	if err != nil || plan.Backend != BackendMPV {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	if len(mpv.loaded) != 1 || mpv.loaded[0].URL != "rtmp://x/live" || mpv.played != 1 {
		t.Fatalf("mpv 后端未真正加载: loaded=%+v played=%d", mpv.loaded, mpv.played)
	}
}

// TestPrepareCapsSessions 每次 Web 播放都在会话表里留一条，供 Fallback
// 降级用；但会话只在降级或关机时清理，长期运行会无限膨胀。本测试断言
// Prepare 超过上限后最旧的会话被淘汰，且最新会话仍能降级。
func TestPrepareCapsSessions(t *testing.T) {
	mpv := &fakePlayer{}
	web := &fakeProxy{}
	c := NewController(nil, web, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }

	var last Plan
	for i := 0; i < maxPlaySessions+10; i++ {
		plan, err := c.Prepare(context.Background(), player.Stream{URL: fmt.Sprintf("https://x/%d.mp4", i), Kind: player.StreamMP4})
		if err != nil {
			t.Fatalf("Prepare %d: %v", i, err)
		}
		last = plan
	}
	c.mu.Lock()
	n := len(c.sessions)
	c.mu.Unlock()
	if n > maxPlaySessions {
		t.Fatalf("Web 播放会话应有上限，得 %d 条（应 <= %d）", n, maxPlaySessions)
	}
	if _, err := c.Fallback(context.Background(), last.ID, 0); err != nil {
		t.Fatalf("最新会话应能降级，got %v", err)
	}
	if mpv.played != 1 {
		t.Fatalf("降级应播放一次，got %d", mpv.played)
	}
}

func TestControllerFallbackLoadsOriginalStream(t *testing.T) {
	proxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, proxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.ID == "" {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	got, err := c.Fallback(context.Background(), plan.ID, 0)
	if err != nil || got.Backend != BackendMPV || len(mpv.loaded) != 1 || mpv.loaded[0].URL != "https://x/live.m3u8" || mpv.played != 1 {
		t.Fatalf("Fallback = %+v, %v, loaded=%+v played=%d", got, err, mpv.loaded, mpv.played)
	}
	if _, err := c.Fallback(context.Background(), plan.ID, 0); err == nil {
		t.Fatal("重复 fallback 应失败")
	}
}

// TestControllerFallbackSeeksToWatchedPosition 断言降级时把网页播放器已看到
// 的位置透传给 mpv——否则 mpv 从视频开头重播，用户丢掉的进度正是本次要修的症状。
func TestControllerFallbackSeeksToWatchedPosition(t *testing.T) {
	proxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, proxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/vod.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.ID == "" {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	got, err := c.Fallback(context.Background(), plan.ID, 123.5)
	if err != nil || got.Backend != BackendMPV {
		t.Fatalf("Fallback = %+v, %v", got, err)
	}
	if len(mpv.seeked) != 1 || mpv.seeked[0] != 123.5 {
		t.Fatalf("降级应定位到已看位置: seeked=%v", mpv.seeked)
	}
}

// TestControllerFallbackZeroStartDoesNotSeek 直播从头接流，start=0 时不该发 seek，
// 避免 mpv 在直播流上执行一次无意义的绝对定位。
func TestControllerFallbackZeroStartDoesNotSeek(t *testing.T) {
	proxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, proxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.ID == "" {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	if _, err := c.Fallback(context.Background(), plan.ID, 0); err != nil {
		t.Fatalf("Fallback = %v", err)
	}
	if len(mpv.seeked) != 0 {
		t.Fatalf("从头播放不应 seek: seeked=%v", mpv.seeked)
	}
}

func TestControllerRequiresMPVForRTMP(t *testing.T) {
	c := NewController(nil, &fakeProxy{}, nil)
	_, err := c.Prepare(context.Background(), player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP})
	if err == nil || !errors.Is(err, ErrMPVUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

// TestControllerPreloadWebUsesIsolatedSession 预载 Web 流应注册独立代理会话：
// 不进入正常播放会话表，也不触碰共享播放器。
// TestProbeHLSCodecReadsChunkedPlaylist 探测只做一次 Body.Read 时，分块
// 传输的首个段可能只含清单头部、CODECS 还在后面的段里：单次 Read
// 拿不到完整清单，HEVC 会被误判成普通 HLS 走 Web，浏览器播不了。
// 修复后应读满 64KB 上限再判定。
func TestProbeHLSCodecReadsChunkedPlaylist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter 不支持 Flush")
			return
		}
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-VERSION:3\n") // 第一块：只有头部，没有 CODECS
		flusher.Flush()
		time.Sleep(100 * time.Millisecond) // 让客户端的首次 Read 只拿到第一块
		fmt.Fprint(w, "#EXT-X-STREAM-INF:BANDWIDTH=8000000,CODECS=\"hvc1.1.6.L120.90\"\nhttps://cdn/x/1080.m3u8\n")
	}))
	defer upstream.Close()

	c := NewController(nil, &fakeProxy{}, &fakePlayer{})
	codec, err := c.probeHLSCodec(context.Background(), player.Stream{URL: upstream.URL + "/master.m3u8", Kind: player.StreamHLS})
	if err != nil {
		t.Fatalf("probeHLSCodec: %v", err)
	}
	if !isHEVC(codec) {
		t.Fatalf("应识别出 HEVC 编码，got %q", codec)
	}
}

func TestControllerPreloadWebUsesIsolatedSession(t *testing.T) {
	mpv := &fakePlayer{}
	web := &fakeProxy{}
	c := NewController(nil, web, mpv)

	plan, err := c.Preload(context.Background(), player.Stream{URL: "https://x/a.mp4", Kind: player.StreamMP4})
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if plan.Backend != BackendWeb || plan.URL == "" || plan.ID == "" {
		t.Fatalf("plan = %#v，期望带代理地址的 Web 预载计划", plan)
	}
	if len(mpv.loaded) != 0 || mpv.played != 0 {
		t.Fatalf("预载不得触碰共享播放器: loaded=%d played=%d", len(mpv.loaded), mpv.played)
	}
	if _, err := c.Fallback(context.Background(), plan.ID, 0); err == nil {
		t.Fatal("预载会话不应进入正常播放会话表")
	}
	if err := c.Release(plan.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(web.released) != 1 || web.released[0] != plan.URL {
		t.Fatalf("released = %v，期望释放预载代理地址 %s", web.released, plan.URL)
	}
	if err := c.Release(plan.ID); err != nil {
		t.Fatalf("重复 Release 应幂等: %v", err)
	}
	if len(web.released) != 1 {
		t.Fatalf("重复 Release 不应重复释放: %v", web.released)
	}
}

// TestControllerPreloadMPVOnlyWarmsUpWithoutPlayer 只能走 mpv 的流，
// 预载只做 Range 网络预热，绝不调用共享播放器的 Load/Play。
func TestControllerPreloadMPVOnlyWarmsUpWithoutPlayer(t *testing.T) {
	var mu sync.Mutex
	var ranges []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer upstream.Close()

	mpv := &fakePlayer{}
	proxy := &fakeProxy{}
	c := NewController(nil, proxy, mpv)
	c.SetWebMSE(false) // 无 MSE：TS 只能走 mpv
	plan, err := c.Preload(context.Background(), player.Stream{URL: upstream.URL + "/v.ts", Kind: player.StreamTS})
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if plan.Backend != BackendMPV {
		t.Fatalf("backend = %v，期望 mpv", plan.Backend)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(ranges)
		mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("预载未发起 Range 预热请求")
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	got := ranges[0]
	mu.Unlock()
	if got != "bytes=0-1" {
		t.Fatalf("Range = %q, want bytes=0-1", got)
	}
	if len(mpv.loaded) != 0 || mpv.played != 0 {
		t.Fatalf("mpv 预载不得调用共享播放器: loaded=%d played=%d", len(mpv.loaded), mpv.played)
	}
	if len(proxy.released) != 0 {
		t.Fatalf("mpv 预载不应注册代理会话: %v", proxy.released)
	}
	if err := c.Release(plan.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

// TestControllerPreloadReleaseCancelsWarmup 释放预载应取消仍在进行的预热。
func TestControllerPreloadReleaseCancelsWarmup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	defer upstream.Close()
	defer close(release)

	mpv := &fakePlayer{}
	c := NewController(nil, &fakeProxy{}, mpv)
	c.SetWebMSE(false)
	plan, err := c.Preload(context.Background(), player.Stream{URL: upstream.URL + "/v.ts", Kind: player.StreamTS})
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("预热未启动")
	}
	if err := c.Release(plan.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(mpv.loaded) != 0 {
		t.Fatal("预载不得调用共享播放器")
	}
}
