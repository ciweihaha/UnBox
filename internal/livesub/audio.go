// Package livesub 直播实时字幕：Whisper 听写英文 + Ollama 翻中文 + 画面叠字幕。
//
// 音频从播放器正在播的直播画面里取（前端 Web Audio 抓 <video> 的声音，
// 与画面天然同步），后端按固定窗口切分，串行走 Whisper 识别、Ollama
// 翻译，译文经 Wails 事件推到前端叠在画面上。
//
// 延迟构成：窗口时长（等一窗采满）+ 识别 + 翻译。窗口越短延迟越低，
// 但太短会把句子切碎、识别和翻译质量都掉下来；默认 2 秒。
package livesub

import (
	"encoding/binary"
	"sync"
)

// SampleRate 是 Whisper 要求的采样率：16kHz 单声道 16 位 PCM。
const SampleRate = 16000

// DefaultWindowSeconds 是默认窗口时长（秒）：2 秒一窗，
// 端到端约 3–4 秒；再短句子会被切碎，翻译质量明显下降。
const DefaultWindowSeconds = 2.0

// WindowBuffer 把连续 PCM 切成固定长度窗口（不重叠）。
// 前端每 0.5 秒推一小段，后端攒满一窗就拿去识别。
type WindowBuffer struct {
	mu            sync.Mutex
	buf           []int16
	windowSamples int
}

// NewWindowBuffer 按窗口秒数建缓冲；秒数非法时回退默认值。
func NewWindowBuffer(windowSeconds float64) *WindowBuffer {
	if windowSeconds <= 0 {
		windowSeconds = DefaultWindowSeconds
	}
	return &WindowBuffer{windowSamples: int(windowSeconds * SampleRate)}
}

// Push 追加一段 PCM 采样。
func (w *WindowBuffer) Push(samples []int16) {
	if len(samples) == 0 {
		return
	}
	w.mu.Lock()
	w.buf = append(w.buf, samples...)
	w.mu.Unlock()
}

// TakeWindow 取出一个完整窗口；不够一窗返回 ok=false。
func (w *WindowBuffer) TakeWindow() ([]int16, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) < w.windowSamples {
		return nil, false
	}
	out := make([]int16, w.windowSamples)
	copy(out, w.buf[:w.windowSamples])
	w.buf = append([]int16(nil), w.buf[w.windowSamples:]...)
	return out, true
}

// PendingWindows 返回当前攒够的整窗个数。
func (w *WindowBuffer) PendingWindows() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.buf) / w.windowSamples
}

// DropOldest 丢掉最旧一窗：处理掉队时用，只保留最新窗口，
// 让字幕追上直播，而不是越攒越落后。
func (w *WindowBuffer) DropOldest() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) < w.windowSamples {
		return
	}
	w.buf = append([]int16(nil), w.buf[w.windowSamples:]...)
}

// Clear 清空缓冲（停播/切台时调用）。
func (w *WindowBuffer) Clear() {
	w.mu.Lock()
	w.buf = nil
	w.mu.Unlock()
}

// EncodeWAV 把 16kHz 单声道 16 位 PCM 编成 WAV 字节，
// whisper-server 不开 --convert 也能直接读。
func EncodeWAV(samples []int16) []byte {
	dataLen := len(samples) * 2
	out := make([]byte, 44+dataLen)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataLen))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16) // PCM 子块长度
	binary.LittleEndian.PutUint16(out[20:22], 1)  // PCM 格式
	binary.LittleEndian.PutUint16(out[22:24], 1)  // 单声道
	binary.LittleEndian.PutUint32(out[24:28], SampleRate)
	binary.LittleEndian.PutUint32(out[28:32], SampleRate*2) // 每秒字节数
	binary.LittleEndian.PutUint16(out[32:34], 2)            // 块对齐
	binary.LittleEndian.PutUint16(out[34:36], 16)           // 位深
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataLen))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[44+i*2:], uint16(s))
	}
	return out
}
