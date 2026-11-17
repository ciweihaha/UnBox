// Package livesub 的音频窗口测试：连续 PCM 切固定窗口、WAV 编码。
package livesub

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWindowBuffer_TakeWindow(t *testing.T) {
	w := NewWindowBuffer(2)
	// 不到一窗不吐。
	w.Push(make([]int16, SampleRate))
	if _, ok := w.TakeWindow(); ok {
		t.Fatal("不到一窗就不该吐出窗口")
	}
	// 再推 1 秒，凑满 2 秒一窗。
	w.Push(make([]int16, SampleRate))
	got, ok := w.TakeWindow()
	if !ok {
		t.Fatal("凑满一窗后应吐出窗口")
	}
	if len(got) != 2*SampleRate {
		t.Fatalf("窗口长度 = %d，想得到 %d", len(got), 2*SampleRate)
	}
	// 取完后没有整窗。
	if _, ok := w.TakeWindow(); ok {
		t.Fatal("取完后不应再有整窗")
	}
}

func TestWindowBuffer_PendingAndDrop(t *testing.T) {
	w := NewWindowBuffer(1)
	w.Push(make([]int16, 3*SampleRate))
	if n := w.PendingWindows(); n != 3 {
		t.Fatalf("待处理窗口 = %d，想得到 3", n)
	}
	// 掉队时丢最旧一窗，只保留最新。
	w.DropOldest()
	if n := w.PendingWindows(); n != 2 {
		t.Fatalf("丢一窗后待处理 = %d，想得到 2", n)
	}
	got, ok := w.TakeWindow()
	if !ok || len(got) != SampleRate {
		t.Fatalf("丢后取窗 = %v,%d", ok, len(got))
	}
}

func TestWindowBuffer_Clear(t *testing.T) {
	w := NewWindowBuffer(1)
	w.Push(make([]int16, 2*SampleRate))
	w.Clear()
	if n := w.PendingWindows(); n != 0 {
		t.Fatalf("清空后待处理 = %d，想得到 0", n)
	}
}

func TestWindowBuffer_SamplesPreserved(t *testing.T) {
	w := NewWindowBuffer(1)
	in := make([]int16, SampleRate)
	for i := range in {
		in[i] = int16(i % 32767)
	}
	w.Push(in)
	got, ok := w.TakeWindow()
	if !ok {
		t.Fatal("应吐出窗口")
	}
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("第 %d 个采样 = %d，想得到 %d", i, got[i], in[i])
		}
	}
}

func TestEncodeWAV(t *testing.T) {
	samples := []int16{0, 1000, -1000, 32767, -32768}
	got := EncodeWAV(samples)
	// WAV 头 44 字节 + 采样数据。
	if len(got) != 44+len(samples)*2 {
		t.Fatalf("WAV 长度 = %d，想得到 %d", len(got), 44+len(samples)*2)
	}
	if string(got[0:4]) != "RIFF" || string(got[8:12]) != "WAVE" || string(got[12:16]) != "fmt " {
		t.Fatal("WAV 头标识不对")
	}
	if string(got[36:40]) != "data" {
		t.Fatal("WAV data 标识不对")
	}
	if got[24] != 0x80 || got[25] != 0x3e { // 16000 小端
		t.Fatalf("采样率字段不对：%v", got[24:28])
	}
	if got[22] != 1 || got[34] != 16 { // 单声道、16 位
		t.Fatal("声道/位深字段不对")
	}
	// 数据区小端 int16。
	for i, want := range samples {
		if gotV := int16(binary.LittleEndian.Uint16(got[44+i*2:])); gotV != want {
			t.Fatalf("第 %d 个采样 = %d，想得到 %d", i, gotV, want)
		}
	}
}

func TestEncodeWAV_Empty(t *testing.T) {
	got := EncodeWAV(nil)
	if len(got) != 44 {
		t.Fatalf("空采样 WAV 长度 = %d，想得到 44", len(got))
	}
	if !bytes.HasPrefix(got, []byte("RIFF")) {
		t.Fatal("空采样也应有合法 WAV 头")
	}
}
