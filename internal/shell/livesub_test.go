// Package shell 的直播实时字幕胶水测试。
package shell

import (
	"context"
	"encoding/base64"
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
	if got.WhisperModel != "base.en" {
		t.Fatalf("默认 WhisperModel = %q，want base.en", got.WhisperModel)
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
	if st.WhisperModel != "base.en" {
		t.Fatalf("状态里的模型 = %q，want base.en", st.WhisperModel)
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
