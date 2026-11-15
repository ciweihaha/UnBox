package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockOllama 模拟 Ollama /api/chat：记录请求，返回固定译文。
func mockOllama(t *testing.T, calls *atomic.Int32, reply string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/chat" {
			t.Errorf("请求路径 = %s，想要 /api/chat", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("请求体不是合法 JSON: %v", err)
		}
		if req["stream"] != false {
			t.Errorf("stream = %v，想要 false（字幕逐条翻译不需要流式）", req["stream"])
		}
		if req["model"] != "qwen2.5:3b" {
			t.Errorf("model = %v，想要 qwen2.5:3b", req["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "qwen2.5:3b",
			"message": map[string]any{
				"role":    "assistant",
				"content": reply,
			},
			"done": true,
		})
	}))
}

func TestTranslateSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := mockOllama(t, &calls, "你好，世界", http.StatusOK)
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
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

func TestTranslateTrimsQuotesAndSpaces(t *testing.T) {
	var calls atomic.Int32
	srv := mockOllama(t, &calls, "  \"你好\" \n", http.StatusOK)
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	got, err := c.Translate(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("Translate 返回错误: %v", err)
	}
	if got != "你好" {
		t.Fatalf("Translate = %q，想要 %q（模型常把译文包在引号里）", got, "你好")
	}
}

func TestTranslateCachesSameText(t *testing.T) {
	var calls atomic.Int32
	srv := mockOllama(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	for i := 0; i < 3; i++ {
		if _, err := c.Translate(context.Background(), "Hello"); err != nil {
			t.Fatalf("第 %d 次 Translate 返回错误: %v", i+1, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP 请求次数 = %d，想要 1（同一条只翻一次）", calls.Load())
	}
}

func TestTranslateEmptySkipsRequest(t *testing.T) {
	var calls atomic.Int32
	srv := mockOllama(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	for _, in := range []string{"", "   ", "\n"} {
		got, err := c.Translate(context.Background(), in)
		if err != nil || got != "" {
			t.Fatalf("Translate(%q) = %q, %v，想要 \"\", nil", in, got, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP 请求次数 = %d，想要 0（空文本不发请求）", calls.Load())
	}
}

func TestTranslateServerError(t *testing.T) {
	var calls atomic.Int32
	srv := mockOllama(t, &calls, "", http.StatusInternalServerError)
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("服务端 500 时 Translate 想要返回错误")
	}
}

func TestTranslateTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":{"content":"你好"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 100*time.Millisecond)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("超时后 Translate 想要返回错误（调用方回退原文）")
	}
}

func TestTranslateContextCanceled(t *testing.T) {
	var calls atomic.Int32
	srv := mockOllama(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Translate(ctx, "Hello"); err == nil {
		t.Fatal("上下文已取消时 Translate 想要返回错误")
	}
}

func TestCheckModelPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("请求路径 = %s，想要 /api/tags", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "qwen2.5:3b"},
				{"name": "llama3.2:3b"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	if err := c.CheckModel(context.Background()); err != nil {
		t.Fatalf("模型已拉取时 CheckModel 想要返回 nil，得到 %v", err)
	}
}

func TestCheckModelAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{{"name": "llama3.2:3b"}},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "qwen2.5:3b", 5*time.Second)
	if err := c.CheckModel(context.Background()); err == nil {
		t.Fatal("模型未拉取时 CheckModel 想要返回错误")
	}
}

func TestCheckModelServerDown(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "qwen2.5:3b", time.Second)
	if err := c.CheckModel(context.Background()); err == nil {
		t.Fatal("Ollama 未运行时 CheckModel 想要返回错误")
	}
}

func TestCleanTranslation(t *testing.T) {
	cases := map[string]string{
		"  你好  ":          "你好",
		"\"你好\"":          "你好",
		"“你好”":            "你好",
		"你好\n":            "你好",
		"翻译：你好":           "你好",
		"Translation: 你好": "你好",
		"你好。":             "你好。",
		"":                "",
	}
	for in, want := range cases {
		if got := cleanTranslation(in); got != want {
			t.Errorf("cleanTranslation(%q) = %q，想要 %q", in, got, want)
		}
	}
}

func TestIsChineseText(t *testing.T) {
	if !isChineseText("你好，世界") {
		t.Error("中文句子应判为中文")
	}
	if !isChineseText("Hello 你好") {
		t.Error("中英混合应判为中文（已有中文就不翻）")
	}
	if isChineseText("Hello, world") {
		t.Error("纯英文不应判为中文")
	}
	if isChineseText("123...!?") {
		t.Error("纯数字标点不应判为中文")
	}
	if isChineseText("") {
		t.Error("空串不应判为中文")
	}
}
