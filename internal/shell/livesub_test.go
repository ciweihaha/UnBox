// Package shell 的直播实时字幕胶水测试。
package shell

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/livesub"
	"github.com/unbox/unbox/internal/store"
)

func newLiveSubService(t *testing.T) *ShellService {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/livesub.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return newPlaybackSettingsService(t, st)
}

// fakeLiveSubServer 是测试用的假 whisper-server：不拉进程，只记起停。
type fakeLiveSubServer struct {
	started bool
	stopped bool
}

func (f *fakeLiveSubServer) Start(ctx context.Context, binPath, modelPath string) (string, error) {
	f.started = true
	return "http://127.0.0.1:17880", nil
}

func (f *fakeLiveSubServer) Stop() error {
	f.stopped = true
	return nil
}

type fakeLiveSubTranscriber struct{ text string }

func (f *fakeLiveSubTranscriber) Transcribe(ctx context.Context, wav []byte) (string, error) {
	return f.text, nil
}

type fakeLiveSubTranslator struct{}

func (f *fakeLiveSubTranslator) Translate(ctx context.Context, text string) (string, error) {
	return "译：" + text, nil
}

// newFakeLiveSubManager 建一个不碰网络/进程的 Manager（测试用）。
func newFakeLiveSubManager(srv *fakeLiveSubServer) func(livesub.ManagerConfig) *livesub.Manager {
	return func(cfg livesub.ManagerConfig) *livesub.Manager {
		cfg.Transcriber = &fakeLiveSubTranscriber{text: "hello"}
		cfg.Translator = &fakeLiveSubTranslator{}
		cfg.Server = srv
		cfg.EnsureAssets = func(ctx context.Context, dir, model string, p livesub.ProgressFunc) (string, string, error) {
			return "/fake/whisper-server", "/fake/model.bin", nil
		}
		return livesub.NewManager(cfg)
	}
}

func TestLiveSubtitleSettingsDefaults(t *testing.T) {
	svc := newLiveSubService(t)
	got := svc.GetLiveSubtitleSettings()
	if got.Enabled {
		t.Fatalf("默认 Enabled = true，want false（直播字幕默认关）")
	}
	if got.WhisperModel != "tiny.en" {
		t.Fatalf("默认 WhisperModel = %q，want tiny.en", got.WhisperModel)
	}
	if got.WindowSeconds != livesub.DefaultWindowSeconds {
		t.Fatalf("默认 WindowSeconds = %v，want %v", got.WindowSeconds, livesub.DefaultWindowSeconds)
	}
}

func TestLiveSubtitleSettingsRoundTrip(t *testing.T) {
	svc := newLiveSubService(t)
	want := LiveSubtitleSettings{Enabled: true, WhisperModel: "tiny.en", WindowSeconds: 1.5}
	if err := svc.SetLiveSubtitleSettings(want); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetLiveSubtitleSettings(); got != want {
		t.Fatalf("round trip = %#v，want %#v", got, want)
	}
}

func TestLiveSubtitleSettingsBadWindowFallsBack(t *testing.T) {
	svc := newLiveSubService(t)
	if err := svc.SetLiveSubtitleSettings(LiveSubtitleSettings{Enabled: true, WhisperModel: "base.en", WindowSeconds: 0}); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetLiveSubtitleSettings(); got.WindowSeconds != livesub.DefaultWindowSeconds {
		t.Fatalf("非法窗口 = %v，want 默认 %v", got.WindowSeconds, livesub.DefaultWindowSeconds)
	}
}

func TestLiveSubtitleStatusNotRunning(t *testing.T) {
	svc := newLiveSubService(t)
	st := svc.LiveSubtitleStatus()
	if st.Running {
		t.Fatal("未启动时 Running 应为 false")
	}
	if st.WhisperModel != "tiny.en" {
		t.Fatalf("状态里的模型 = %q，want tiny.en", st.WhisperModel)
	}
}

func TestPushLiveAudioChunkBadInput(t *testing.T) {
	svc := newLiveSubService(t)
	if err := svc.PushLiveAudioChunk("!!!not-base64!!!"); err == nil {
		t.Fatal("非法 base64 应返回错误")
	}
	// 奇数长度不是合法的 16 位采样。
	odd := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
	if err := svc.PushLiveAudioChunk(odd); err == nil {
		t.Fatal("奇数长度音频应返回错误")
	}
	// 未启动时合法音频直接丢弃，不报错。
	ok := base64.StdEncoding.EncodeToString([]byte{0x00, 0x00, 0x01, 0x00})
	if err := svc.PushLiveAudioChunk(ok); err != nil {
		t.Fatalf("未启动时推音频 err = %v，want nil（直接丢弃）", err)
	}
}

func TestStartStopLiveSubtitlesWithFakeManager(t *testing.T) {
	svc := newLiveSubService(t)
	srv := &fakeLiveSubServer{}
	svc.liveSubFactory = newFakeLiveSubManager(srv)
	if err := svc.SetLiveSubtitleSettings(LiveSubtitleSettings{Enabled: true, WhisperModel: "base.en", WindowSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartLiveSubtitles(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !srv.started {
		t.Fatal("启动时应拉起 whisper-server")
	}
	if st := svc.LiveSubtitleStatus(); !st.Running {
		t.Fatalf("启动后状态 Running = false，want true（%+v）", st)
	}
	// 推一窗音频，应出一条字幕（经 Manager 的假识别+假翻译）。
	samples := make([]byte, 2*livesub.SampleRate)
	if err := svc.PushLiveAudioChunk(base64.StdEncoding.EncodeToString(samples)); err != nil {
		t.Fatalf("推音频失败: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st := svc.LiveSubtitleStatus(); st.WindowsProcessed >= 1 {
			if st.LastSubtitle != "译：hello" {
				t.Fatalf("字幕 = %q，want 译：hello", st.LastSubtitle)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("3 秒内应处理完一窗")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	if !srv.stopped {
		t.Fatal("停止时应停掉 whisper-server")
	}
	if st := svc.LiveSubtitleStatus(); st.Running {
		t.Fatal("停止后 Running 应为 false")
	}
}

func TestStopLiveSubtitlesNotRunningIsNoop(t *testing.T) {
	svc := newLiveSubService(t)
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatalf("未启动时停止 err = %v，want nil", err)
	}
}

// blockingLiveSubFactory 造一个会在 EnsureAssets 里卡住的假管线：
// entered 在第一次进入 EnsureAssets 时关闭，release 放行下载，
// ctx 被取消时立即失败（模拟停播打断下载）。
func blockingLiveSubFactory(srv *fakeLiveSubServer, entered, release chan struct{}) func(livesub.ManagerConfig) *livesub.Manager {
	var once sync.Once
	return func(cfg livesub.ManagerConfig) *livesub.Manager {
		cfg.Transcriber = &fakeLiveSubTranscriber{text: "hello"}
		cfg.Translator = &fakeLiveSubTranslator{}
		cfg.Server = srv
		cfg.EnsureAssets = func(ctx context.Context, dir, model string, p livesub.ProgressFunc) (string, string, error) {
			once.Do(func() { close(entered) })
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-release:
				return "/fake/whisper-server", "/fake/model.bin", nil
			}
		}
		return livesub.NewManager(cfg)
	}
}

func enableLiveSubForTest(t *testing.T, svc *ShellService) {
	t.Helper()
	if err := svc.SetLiveSubtitleSettings(LiveSubtitleSettings{Enabled: true, WhisperModel: "tiny.en", WindowSeconds: 1.5}); err != nil {
		t.Fatal(err)
	}
}

// TestStartLiveSubtitlesStoppedStartDoesNotDropRestart 复现问题 1：
// 首次播放的启动卡在下载时被一次过期停止打断，随后新的播放请求重新启动，
// 管线必须跑起来——旧代码里第二次 Start 被 liveSubStarting 守卫静默吞掉，
// 第一次启动结束又因代次变化被停掉，之后再也没人重试，首次播放全程没字幕。
func TestStartLiveSubtitlesStoppedStartDoesNotDropRestart(t *testing.T) {
	svc := newLiveSubService(t)
	srv := &fakeLiveSubServer{}
	entered := make(chan struct{})
	release := make(chan struct{})
	svc.liveSubFactory = blockingLiveSubFactory(srv, entered, release)
	enableLiveSubForTest(t, svc)

	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.StartLiveSubtitles() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("第一次启动 5 秒内没进 EnsureAssets")
	}
	// 切台/过期请求把第一次启动停掉。
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	// 新的播放请求重新启动（prepareChannel 的 ensure 走这条路）。
	secondDone := make(chan error, 1)
	go func() { secondDone <- svc.StartLiveSubtitles() }()
	// 放行下载：第一次启动应被取消打断，第二次启动随后成功。
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if st := svc.LiveSubtitleStatus(); st.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("第二次启动后 5 秒内管线应跑起来（旧代码：守卫吞掉重启，全程没字幕）")
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("第一次启动 err = %v，want nil（被取消是正常停止）", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("第一次启动没返回")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("第二次启动 err = %v，want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("第二次启动没返回")
	}
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatal(err)
	}
}

// TestStartLiveSubtitlesWaiterDoesNotResurrectAfterStop：
// 等待中的第二次启动若在等待期间又被停掉，必须作废、不能复活管线。
func TestStartLiveSubtitlesWaiterDoesNotResurrectAfterStop(t *testing.T) {
	svc := newLiveSubService(t)
	srv := &fakeLiveSubServer{}
	entered := make(chan struct{})
	release := make(chan struct{})
	svc.liveSubFactory = blockingLiveSubFactory(srv, entered, release)
	enableLiveSubForTest(t, svc)

	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.StartLiveSubtitles() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("第一次启动 5 秒内没进 EnsureAssets")
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- svc.StartLiveSubtitles() }()
	// 让第二次启动确实进入等待（200ms 足够 goroutine 调度）。
	time.Sleep(200 * time.Millisecond)
	// 停播：两次启动都应作废。
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	close(release)
	for _, ch := range []chan error{firstDone, secondDone} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("启动 err = %v，want nil（被停是正常情况）", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("启动没返回")
		}
	}
	if st := svc.LiveSubtitleStatus(); st.Running {
		t.Fatal("停播后管线不应跑起来（等待中的启动复活了管线）")
	}
}

// TestStartLiveSubtitlesFailureSurfacedInStatus：
// 启动失败时状态里要有错误原因——旧代码失败后 liveSub 被置 nil，
// LiveSubtitleStatus 只从活着的 Manager 取错误，设置页永远看不到原因。
func TestStartLiveSubtitlesFailureSurfacedInStatus(t *testing.T) {
	svc := newLiveSubService(t)
	attempts := 0
	svc.liveSubFactory = func(cfg livesub.ManagerConfig) *livesub.Manager {
		cfg.Transcriber = &fakeLiveSubTranscriber{text: "hello"}
		cfg.Translator = &fakeLiveSubTranslator{}
		cfg.Server = &fakeLiveSubServer{}
		cfg.EnsureAssets = func(ctx context.Context, dir, model string, p livesub.ProgressFunc) (string, string, error) {
			attempts++
			if attempts == 1 {
				return "", "", fmt.Errorf("下载语音识别模型失败: 模拟断网")
			}
			return "/fake/whisper-server", "/fake/model.bin", nil
		}
		return livesub.NewManager(cfg)
	}
	enableLiveSubForTest(t, svc)

	if err := svc.StartLiveSubtitles(); err == nil {
		t.Fatal("第一次启动应返回错误")
	}
	if st := svc.LiveSubtitleStatus(); st.Error == "" {
		t.Fatal("启动失败后状态里应有错误原因（否则设置页看不到、日志也无处查）")
	}
	if err := svc.StartLiveSubtitles(); err != nil {
		t.Fatalf("重试启动失败: %v", err)
	}
	if st := svc.LiveSubtitleStatus(); !st.Running || st.Error != "" {
		t.Fatalf("重试成功后 Running=%v Error=%q，want Running=true Error 为空", st.Running, st.Error)
	}
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureLiveSubtitlesForChannelRetriesFailedStart：
// ensure 的后台启动失败一次后应自动重试，不用用户手动重切台。
func TestEnsureLiveSubtitlesForChannelRetriesFailedStart(t *testing.T) {
	oldDelay := liveSubRetryDelay
	liveSubRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { liveSubRetryDelay = oldDelay })

	svc := newLiveSubService(t)
	// attempts 由 EnsureAssets 在后台重试 goroutine 里写、测试主 goroutine 读，
	// 必须用 atomic，否则 go test -race 报 data race。
	var attempts atomic.Int32
	svc.liveSubFactory = func(cfg livesub.ManagerConfig) *livesub.Manager {
		cfg.Transcriber = &fakeLiveSubTranscriber{text: "hello"}
		cfg.Translator = &fakeLiveSubTranslator{}
		cfg.Server = &fakeLiveSubServer{}
		cfg.EnsureAssets = func(ctx context.Context, dir, model string, p livesub.ProgressFunc) (string, string, error) {
			n := attempts.Add(1)
			if n < 3 {
				return "", "", fmt.Errorf("模拟下载失败（第 %d 次）", n)
			}
			return "/fake/whisper-server", "/fake/model.bin", nil
		}
		return livesub.NewManager(cfg)
	}
	enableLiveSubForTest(t, svc)

	svc.ensureLiveSubtitlesForChannel("web")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if st := svc.LiveSubtitleStatus(); st.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ensure 后 5 秒内管线应跑起来（已尝试 %d 次）", attempts.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if attempts.Load() != 3 {
		t.Fatalf("启动尝试次数 = %d，want 3（前两次失败、第三次成功）", attempts.Load())
	}
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureLiveSubtitlesForChannelStopAbortsRetry：
// 停播/切台后 ensure 的重试必须停，不应复活管线。
func TestEnsureLiveSubtitlesForChannelStopAbortsRetry(t *testing.T) {
	oldDelay := liveSubRetryDelay
	liveSubRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { liveSubRetryDelay = oldDelay })

	svc := newLiveSubService(t)
	// attempts 由 EnsureAssets 在后台重试 goroutine 里写、测试主 goroutine 读，
	// 必须用 atomic，否则 go test -race 报 data race。
	var attempts atomic.Int32
	svc.liveSubFactory = func(cfg livesub.ManagerConfig) *livesub.Manager {
		cfg.Transcriber = &fakeLiveSubTranscriber{text: "hello"}
		cfg.Translator = &fakeLiveSubTranslator{}
		cfg.Server = &fakeLiveSubServer{}
		cfg.EnsureAssets = func(ctx context.Context, dir, model string, p livesub.ProgressFunc) (string, string, error) {
			n := attempts.Add(1)
			return "", "", fmt.Errorf("模拟下载失败（第 %d 次）", n)
		}
		return livesub.NewManager(cfg)
	}
	enableLiveSubForTest(t, svc)

	svc.ensureLiveSubtitlesForChannel("web")
	// 等重试确实跑起来（至少两次尝试）。
	deadline := time.Now().Add(5 * time.Second)
	for {
		if attempts.Load() >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ensure 后 5 秒内应至少尝试 2 次，实际 %d 次", attempts.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := svc.StopLiveSubtitles(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	stoppedAt := attempts.Load()
	time.Sleep(300 * time.Millisecond)
	if attempts.Load() != stoppedAt {
		t.Fatalf("停播后又尝试了 %d 次（停时 %d 次）：重试应停，不应复活管线", attempts.Load(), stoppedAt)
	}
	if st := svc.LiveSubtitleStatus(); st.Running {
		t.Fatal("停播后管线不应跑起来")
	}
}

// TestEnsureLiveSubtitlesForChannelNonWebIsNoop：mpv 后端没有 <video> 可抓音频，
// ensure 不应启动管线。
func TestEnsureLiveSubtitlesForChannelNonWebIsNoop(t *testing.T) {
	svc := newLiveSubService(t)
	started := false
	svc.liveSubFactory = func(cfg livesub.ManagerConfig) *livesub.Manager {
		started = true
		return newFakeLiveSubManager(&fakeLiveSubServer{})(cfg)
	}
	enableLiveSubForTest(t, svc)
	svc.ensureLiveSubtitlesForChannel("mpv")
	time.Sleep(200 * time.Millisecond)
	if started {
		t.Fatal("mpv 后端不应启动字幕管线")
	}
	if st := svc.LiveSubtitleStatus(); st.Running {
		t.Fatal("mpv 后端状态不应为 Running")
	}
}
