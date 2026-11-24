package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/httpx"
)

// mockWorkersAI 模拟 Cloudflare Workers AI 的翻译接口：记录请求，返回固定译文。
func mockWorkersAI(t *testing.T, calls *atomic.Int32, reply string, success bool, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/accounts/test-acct/ai/run/@cf/meta/m2m100-1.2b" {
			t.Errorf("请求路径 = %s，想要 /accounts/test-acct/ai/run/@cf/meta/m2m100-1.2b", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q，想要 Bearer test-token", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("请求体不是合法 JSON: %v", err)
		}
		if req["source_lang"] != "english" || req["target_lang"] != "chinese" {
			t.Errorf("语言 = %v/%v，想要 english/chinese", req["source_lang"], req["target_lang"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": success,
			"result": map[string]any{
				"translated_text": reply,
			},
			"errors": []any{},
		})
	}))
}

func TestCFTranslateSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "你好，世界", true, http.StatusOK)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
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

func TestCFTranslateSkipsEmptyAndChinese(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "你好", true, http.StatusOK)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
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
		t.Fatalf("HTTP 请求次数 = %d，想要 0（空文本与中文不发请求）", calls.Load())
	}
}

func TestCFTranslateCachesSameText(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "你好", true, http.StatusOK)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
	for i := 0; i < 3; i++ {
		if _, err := c.Translate(context.Background(), "Hello"); err != nil {
			t.Fatalf("第 %d 次 Translate 返回错误: %v", i+1, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP 请求次数 = %d，想要 1（同一条只翻一次）", calls.Load())
	}
}

func TestCFTranslateServerError(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "", false, http.StatusOK)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatalf("Translate 成功，想要错误（接口返回 success=false）")
	}
}

func TestCFTranslateHTTPError(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "", true, http.StatusForbidden)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatalf("Translate 成功，想要错误（接口返回 HTTP 403）")
	}
}

func TestCFTranslateEmptyResult(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "   ", true, http.StatusOK)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatalf("Translate 成功，想要错误（接口返回空译文）")
	}
}

func TestCFTranslateTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"result":{"translated_text":"你好"}}`))
	}))
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 50*time.Millisecond)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatalf("Translate 成功，想要超时错误")
	}
}

// Check 用一句 Hello 探测账号与令牌是否可用：
// 账号/令牌错会直接报接口错误，调用方据此提示用户去设置页检查。
func TestCFCheck(t *testing.T) {
	var calls atomic.Int32
	srv := mockWorkersAI(t, &calls, "你好", true, http.StatusOK)
	defer srv.Close()

	c := NewCFClient("test-acct", "test-token", srv.URL, 5*time.Second)
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP 请求次数 = %d，想要 1", calls.Load())
	}
}

func TestCFCheckMissingCredentials(t *testing.T) {
	c := NewCFClient("", "", "", 5*time.Second)
	if err := c.Check(context.Background()); err == nil {
		t.Fatalf("Check 成功，想要错误（账号与令牌为空）")
	}
}

// TestCFTranslateUsesEnvProxy：与 Azure 同理，Workers AI 翻译也要走统一代理选择。
func TestCFTranslateUsesEnvProxy(t *testing.T) {
	c := NewCFClient("test-acct", "test-token", "http://cf-test.invalid", 5*time.Second)
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("Cloudflare 客户端没有显式传输层（Transport = %v）", c.http.Transport)
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(httpx.ProxyForRequest).Pointer() {
		t.Fatalf("Cloudflare 客户端没走统一代理选择函数 httpx.ProxyForRequest")
	}

	// 行为烟囱：经环境变量代理拿到译文。
	var viaProxy atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]any{"translated_text": "你好，世界"},
			"errors":  []any{},
		})
	}))
	defer proxy.Close()
	setTranslateTestProxyEnv(t, proxy.URL)

	got, err := c.Translate(context.Background(), "Hello, world")
	if err != nil {
		t.Fatalf("Translate 报错: %v", err)
	}
	if got != "你好，世界" {
		t.Fatalf("Translate = %q，想要 %q", got, "你好，世界")
	}
	if viaProxy.Load() != 1 {
		t.Fatalf("经代理的请求次数 = %d，想要 1", viaProxy.Load())
	}
}
