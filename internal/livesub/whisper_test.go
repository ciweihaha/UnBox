// Package livesub 的 Whisper 客户端测试：调 whisper-server 的 /inference。
package livesub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWhisperClient_Transcribe(t *testing.T) {
	var gotContentType, gotFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference" {
			t.Errorf("请求路径 = %s，想得到 /inference", r.URL.Path)
		}
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("解析 multipart 失败: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("file 字段缺失: %v", err)
		}
		defer f.Close()
		gotFile = hdr.Filename
		// 语言字段应为 en（省掉语言检测，快且准）。
		if got := r.FormValue("language"); got != "en" {
			t.Errorf("language = %q，想得到 en", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "Hello world"})
	}))
	defer srv.Close()

	c := NewWhisperClient(srv.URL, time.Second)
	text, err := c.Transcribe(t.Context(), []byte("fake-wav"))
	if err != nil {
		t.Fatalf("识别失败: %v", err)
	}
	if text != "Hello world" {
		t.Fatalf("识别文本 = %q，想得到 Hello world", text)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("Content-Type = %q，应为 multipart", gotContentType)
	}
	if gotFile != "audio.wav" {
		t.Fatalf("文件名 = %q，想得到 audio.wav", gotFile)
	}
}

func TestWhisperClient_TranscribeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad audio"))
	}))
	defer srv.Close()
	c := NewWhisperClient(srv.URL, time.Second)
	if _, err := c.Transcribe(t.Context(), []byte("x")); err == nil {
		t.Fatal("HTTP 400 应返回错误")
	}
}

func TestWhisperClient_TranscribeEmptyWAV(t *testing.T) {
	c := NewWhisperClient("http://127.0.0.1:1", time.Second)
	if _, err := c.Transcribe(t.Context(), nil); err == nil {
		t.Fatal("空音频应返回错误，不发请求")
	}
}

func TestWhisperClient_WaitReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewWhisperClient(srv.URL, time.Second)
	if err := c.WaitReady(t.Context(), 2*time.Second); err != nil {
		t.Fatalf("服务已起时 WaitReady 应成功: %v", err)
	}
}

func TestWhisperClient_WaitReadyTimeout(t *testing.T) {
	c := NewWhisperClient("http://127.0.0.1:1", 50*time.Millisecond)
	if err := c.WaitReady(t.Context(), 200*time.Millisecond); err == nil {
		t.Fatal("服务没起时 WaitReady 应超时报错")
	}
}
