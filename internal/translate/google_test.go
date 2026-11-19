package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockGoogle 模拟 Cloud Translation 接口：记录请求，返回固定译文。
func mockGoogle(t *testing.T, calls *atomic.Int32, reply string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/language/translate/v2" {
			t.Errorf("请求路径 = %s，想要 /language/translate/v2", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key 参数 = %q，想要 test-key", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("请求体不是合法 JSON: %v", err)
		}
		if req["source"] != "en" || req["target"] != "zh-CN" {
			t.Errorf("语言 = %v/%v，想要 en/zh-CN", req["source"], req["target"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"translations": []map[string]any{{"translatedText": reply}},
			},
		})
	}))
}

func TestGoogleTranslateSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := mockGoogle(t, &calls, "你好，世界", http.StatusOK)
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 5*time.Second)
	got, err := c.Translate(context.Background(), "Hello, world")
	if err != nil {
		t.Fatalf("Translate 返回错误: %v", err)
	}
	if got != "你好，世界" {
		t.Fatalf("Translate = %q，想要 %q", got, "你好，世界")
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP 请求次数 = %d，想要 1", calls.Load())
	}
}

func TestGoogleTranslateUnescapesEntities(t *testing.T) {
	var calls atomic.Int32
	srv := mockGoogle(t, &calls, "你好 &amp; 再见", http.StatusOK)
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 5*time.Second)
	got, err := c.Translate(context.Background(), "Hello & goodbye")
	if err != nil {
		t.Fatalf("Translate 返回错误: %v", err)
	}
	if got != "你好 & 再见" {
		t.Fatalf("Translate = %q，想要 %q（HTML 转义应先反转义）", got, "你好 & 再见")
	}
}

func TestGoogleTranslateSkipsEmptyAndChinese(t *testing.T) {
	var calls atomic.Int32
	srv := mockGoogle(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 5*time.Second)
	for _, in := range []string{"", "   ", "你好世界"} {
		got, err := c.Translate(context.Background(), in)
		want := in
		if strings.TrimSpace(in) == "" {
			want = ""
		}
		if err != nil || got != want {
			t.Fatalf("Translate(%q) = %q, %v，想要 %q, nil", in, got, err, want)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP 请求次数 = %d，想要 0", calls.Load())
	}
}

func TestGoogleTranslateCachesSameText(t *testing.T) {
	var calls atomic.Int32
	srv := mockGoogle(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 5*time.Second)
	for i := 0; i < 3; i++ {
		if _, err := c.Translate(context.Background(), "Hello"); err != nil {
			t.Fatalf("Translate 返回错误: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP 请求次数 = %d，想要 1（同一条只翻一次）", calls.Load())
	}
}

func TestGoogleTranslateMissingKey(t *testing.T) {
	c := NewGoogleClient("", "http://127.0.0.1:1", 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("未填 Key 应返回错误")
	}
}

func TestGoogleTranslateServerError(t *testing.T) {
	var calls atomic.Int32
	srv := mockGoogle(t, &calls, "", http.StatusForbidden)
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("HTTP 403 应返回错误")
	}
}

func TestGoogleTranslateTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 100*time.Millisecond)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("超时应返回错误")
	}
}

func TestGoogleCheck(t *testing.T) {
	var calls atomic.Int32
	srv := mockGoogle(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewGoogleClient("test-key", srv.URL, 5*time.Second)
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if err := NewGoogleClient("", srv.URL, 5*time.Second).Check(context.Background()); err == nil {
		t.Fatal("未填 Key 时 Check 应返回错误")
	}
}
