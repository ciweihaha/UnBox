// Package livesub 的真实管线集成测试：
// 用真 whisper-server + 真 base.en 模型跑一遍识别→翻译→推送，
// 测端到端延迟。需要测试资源（程序+模型+一段英文 WAV），
// 由环境变量 UNBOX_LIVESUB_TEST_ASSETS 指向资源根目录，
// 目录结构与运行时缓存一致：
//
//	<assets>/bin/whisper-server(.exe) 与 DLL
//	<assets>/models/ggml-base.en.bin
//	<assets>/speech.wav（16kHz 单声道 16 位 PCM）
//
// 未设置或资源不全时跳过（CI 默认跳过，不拉 148MB 模型）。
package livesub

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManager_RealWhisperServer(t *testing.T) {
	assets := os.Getenv("UNBOX_LIVESUB_TEST_ASSETS")
	if assets == "" {
		t.Skip("未设置 UNBOX_LIVESUB_TEST_ASSETS，跳过真实管线测试")
	}
	binName := "whisper-server"
	modelPath := filepath.Join(assets, "models", "ggml-base.en.bin")
	if _, err := os.Stat(filepath.Join(assets, "bin", binName)); err != nil {
		t.Skipf("测试资源缺程序：%v", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("测试资源缺模型：%v", err)
	}
	wavPath := filepath.Join(assets, "speech.wav")
	samples, err := readWAV16kMono(wavPath)
	if err != nil {
		t.Skipf("测试资源缺音频：%v", err)
	}
	if len(samples) < SampleRate {
		t.Fatalf("测试音频太短：%d 采样", len(samples))
	}

	emitted := &emittedSubtitles{}
	m := NewManager(ManagerConfig{
		WindowSeconds: 2,
		WhisperModel:  "base.en",
		DataDir:       assets,
		Translator:    &fakeTranslator{},
		EnsureAssets: func(ctx context.Context, dir, model string, p ProgressFunc) (string, string, error) {
			return filepath.Join(assets, "bin", binName), modelPath, nil
		},
		Emit: func(text string, latency time.Duration) {
			emitted.add(emittedSubtitle{Text: text, Latency: latency})
		},
	})
	start := time.Now()
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("启动真实管线失败: %v", err)
	}
	defer m.Stop()
	t.Logf("whisper-server 启动耗时 %v", time.Since(start))

	// 只喂前 4 秒音频（2 窗）。弱 CPU 上识别很慢，掉窗是正常的保活行为；
	// 本测试只验证真管线能跑通（程序拉起→识别→翻译→推送），
	// 延迟与准确率以云电脑实测为准，这里只记录、不硬卡。
	feed := samples
	if len(feed) > 4*SampleRate {
		feed = feed[:4*SampleRate]
	}
	chunk := SampleRate / 2
	for i := 0; i+chunk <= len(feed); i += chunk {
		m.PushAudio(feed[i : i+chunk])
	}
	if !waitEmittedLong(t, emitted, 1, 60*time.Second) {
		t.Fatalf("60 秒内应出第一条字幕，统计：%+v", m.Stats())
	}
	st := m.Stats()
	t.Logf("首条字幕：%q，延迟 %v，识别 %v，翻译 %v，统计：%+v",
		emitted.get(0).Text, st.LastLatency, st.LastASR, st.LastTranslate, st)
	if emitted.get(0).Text == "" {
		t.Errorf("字幕为空")
	}
	if st.Error != "" {
		t.Errorf("管线报错：%s", st.Error)
	}
}

// waitEmittedLong 等 n 条字幕出现（最多 timeout），超时返回 false。
func waitEmittedLong(t *testing.T, e *emittedSubtitles, n int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.len() >= n {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// readWAV16kMono 读 16kHz 单声道 16 位 PCM WAV，返回采样。
// 只认标准 44 字节头（测试资源就是这个格式）。
func readWAV16kMono(path string) ([]int16, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, os.ErrInvalid
	}
	data := raw[44:]
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out, nil
}
