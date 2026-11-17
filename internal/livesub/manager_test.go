// Package livesub 的 Manager 测试：串行处理窗口、掉队追赶、延迟统计。
package livesub

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeTranscriber struct {
	mu    sync.Mutex
	text  string
	err   error
	calls int
	delay time.Duration
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, wav []byte) (string, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	text := f.text
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
	}
	if err != nil {
		return "", err
	}
	return text, nil
}

type fakeTranslator struct {
	mu    sync.Mutex
	calls int
	fn    func(string) (string, error)
}

func (f *fakeTranslator) Translate(ctx context.Context, text string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(text)
	}
	return "中文：" + text, nil
}

type fakeServer struct {
	mu      sync.Mutex
	started bool
	stopped bool
}

func (f *fakeServer) Start(ctx context.Context, binPath, modelPath string) (string, error) {
	f.mu.Lock()
	f.started = true
	f.mu.Unlock()
	return "http://127.0.0.1:17880", nil
}

func (f *fakeServer) Stop() error {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
	return nil
}

// emittedSubtitles 是测试用的字幕收集器（带锁）。
type emittedSubtitles struct {
	mu    sync.Mutex
	items []emittedSubtitle
}

func (e *emittedSubtitles) add(s emittedSubtitle) {
	e.mu.Lock()
	e.items = append(e.items, s)
	e.mu.Unlock()
}

func (e *emittedSubtitles) len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.items)
}

func (e *emittedSubtitles) get(i int) emittedSubtitle {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.items[i]
}

func newTestManager(t *testing.T, tr *fakeTranscriber, tl *fakeTranslator) (*Manager, *fakeServer, *emittedSubtitles) {
	t.Helper()
	srv := &fakeServer{}
	emitted := &emittedSubtitles{}
	m := NewManager(ManagerConfig{
		WindowSeconds: 1,
		WhisperModel:  "base.en",
		DataDir:       t.TempDir(),
		Transcriber:   tr,
		Translator:    tl,
		Server:        srv,
		EnsureAssets: func(ctx context.Context, dir, model string, p ProgressFunc) (string, string, error) {
			return "/fake/whisper-server", "/fake/ggml-base.en.bin", nil
		},
		Emit: func(text string, latency time.Duration) {
			emitted.add(emittedSubtitle{Text: text, Latency: latency})
		},
	})
	return m, srv, emitted
}

// waitEmitted 等一条字幕出现（最多 3 秒），超时返回 false。
func waitEmitted(t *testing.T, e *emittedSubtitles, n int) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e.len() >= n {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func TestManager_StartStop(t *testing.T) {
	m, srv, emitted := newTestManager(t, &fakeTranscriber{text: "hi"}, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !srv.started {
		t.Fatal("启动时应拉起 whisper-server")
	}
	if !m.Running() {
		t.Fatal("启动后 Running 应为真")
	}
	m.PushAudio(make([]int16, SampleRate))
	if !waitEmitted(t, emitted, 1) {
		t.Fatal("推音频后 3 秒内应出字幕")
	}
	if got := emitted.get(0).Text; got != "中文：hi" {
		t.Fatalf("字幕 = %q，想得到 中文：hi", got)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	if !srv.stopped {
		t.Fatal("停止时应停掉 whisper-server")
	}
	if m.Running() {
		t.Fatal("停止后 Running 应为假")
	}
}

func TestManager_EmptyTranscriptionSkipped(t *testing.T) {
	m, _, emitted := newTestManager(t, &fakeTranscriber{text: "   "}, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	m.PushAudio(make([]int16, SampleRate))
	time.Sleep(500 * time.Millisecond)
	if emitted.len() != 0 {
		t.Fatalf("识别为空时不应出字幕，出了 %d 条", emitted.len())
	}
}

func TestManager_BlankAudioHallucinationSkipped(t *testing.T) {
	m, _, emitted := newTestManager(t, &fakeTranscriber{text: "[BLANK_AUDIO]"}, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	m.PushAudio(make([]int16, SampleRate))
	time.Sleep(500 * time.Millisecond)
	if emitted.len() != 0 {
		t.Fatalf("静音幻觉不应出字幕，出了 %d 条", emitted.len())
	}
}

func TestManager_DuplicateSkipped(t *testing.T) {
	m, _, emitted := newTestManager(t, &fakeTranscriber{text: "same line"}, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	m.PushAudio(make([]int16, SampleRate))
	m.PushAudio(make([]int16, SampleRate))
	if !waitEmitted(t, emitted, 1) {
		t.Fatal("应出 1 条字幕")
	}
	time.Sleep(800 * time.Millisecond)
	if emitted.len() != 1 {
		t.Fatalf("重复文本只应出 1 条字幕，出了 %d 条", emitted.len())
	}
}

func TestManager_TranslateFailureFallsBackToEnglish(t *testing.T) {
	tl := &fakeTranslator{fn: func(string) (string, error) { return "", errors.New("模型挂了") }}
	m, _, emitted := newTestManager(t, &fakeTranscriber{text: "hello"}, tl)
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	m.PushAudio(make([]int16, SampleRate))
	if !waitEmitted(t, emitted, 1) {
		t.Fatal("翻译失败也应出一条字幕（英文回退）")
	}
	if got := emitted.get(0).Text; got != "hello" {
		t.Fatalf("翻译失败应回退显示英文，得到了 %q", got)
	}
}

func TestManager_TranscribeFailureSkipsWindow(t *testing.T) {
	tr := &fakeTranscriber{err: errors.New("识别挂了")}
	m, _, emitted := newTestManager(t, tr, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	// 第一窗识别失败：不应出字幕。
	m.PushAudio(make([]int16, SampleRate))
	time.Sleep(600 * time.Millisecond)
	if emitted.len() != 0 {
		t.Fatalf("识别失败时不应出字幕，出了 %d 条", emitted.len())
	}
	// 恢复识别，第二窗应正常出字幕。
	tr.mu.Lock()
	tr.err = nil
	tr.text = "recovered"
	tr.mu.Unlock()
	m.PushAudio(make([]int16, SampleRate))
	if !waitEmitted(t, emitted, 1) {
		t.Fatal("恢复后应出字幕")
	}
}

func TestManager_LatencyStats(t *testing.T) {
	tr := &fakeTranscriber{text: "hi", delay: 100 * time.Millisecond}
	m, _, _ := newTestManager(t, tr, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	m.PushAudio(make([]int16, SampleRate))
	deadline := time.Now().Add(3 * time.Second)
	for {
		st := m.Stats()
		if st.WindowsProcessed >= 1 {
			if st.LastLatency <= 0 {
				t.Fatalf("延迟统计应大于 0，得到了 %v", st.LastLatency)
			}
			// 窗口 1 秒 + 识别约 100ms：延迟应在 1–3 秒之间。
			if st.LastLatency < time.Second || st.LastLatency > 3*time.Second {
				t.Fatalf("延迟 = %v，应在 1–3 秒之间", st.LastLatency)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("3 秒内应处理完一窗")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestManager_StartTwiceIsNoop(t *testing.T) {
	m, _, _ := newTestManager(t, &fakeTranscriber{text: "hi"}, &fakeTranslator{})
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer m.Stop()
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("重复启动应直接成功: %v", err)
	}
}

func TestManager_CleanText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[BLANK_AUDIO]", ""},
		{"hello [音乐] world", "hello world"},
		{"(applause) hello", "hello"},
		{"  hello world  ", "hello world"},
		{"[BLANK_AUDIO] hello", "hello"},
	}
	for _, c := range cases {
		if got := cleanTranscription(c.in); got != c.want {
			t.Errorf("cleanTranscription(%q) = %q，想得到 %q", c.in, got, c.want)
		}
	}
}
