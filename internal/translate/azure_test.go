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

// mockAzure 模拟 Azure Translator 接口：记录请求，返回固定译文。
func mockAzure(t *testing.T, calls *atomic.Int32, reply string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/translate" {
			t.Errorf("请求路径 = %s，想要 /translate", r.URL.Path)
		}
		if q := r.URL.Query(); q.Get("api-version") != "3.0" || q.Get("from") != "en" || q.Get("to") != "zh-Hans" {
			t.Errorf("查询参数 = %v，想要 api-version=3.0&from=en&to=zh-Hans", r.URL.RawQuery)
		}
		if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != "test-key" {
			t.Errorf("Ocp-Apim-Subscription-Key = %q，想要 test-key", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req []map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("请求体不是合法 JSON: %v", err)
		}
		if len(req) != 1 || req[0]["Text"] == "" {
			t.Errorf("请求体 = %s，想要一条带 Text 的请求", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"translations": []map[string]any{{"text": reply, "to": "zh-Hans"}}},
		})
	}))
}

func TestAzureTranslateSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := mockAzure(t, &calls, "你好，世界", http.StatusOK)
	defer srv.Close()

	c := NewAzureClient("test-key", "", srv.URL, 5*time.Second)
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

func TestAzureTranslateSendsRegionWhenSet(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("Ocp-Apim-Subscription-Region"); got != "eastasia" {
			t.Errorf("Ocp-Apim-Subscription-Region = %q，想要 eastasia", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"translations": []map[string]any{{"text": "你好", "to": "zh-Hans"}}},
		})
	}))
	defer srv.Close()

	c := NewAzureClient("test-key", "eastasia", srv.URL, 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err != nil {
		t.Fatalf("Translate 返回错误: %v", err)
	}
}

func TestAzureTranslateSkipsEmptyAndChinese(t *testing.T) {
	var calls atomic.Int32
	srv := mockAzure(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewAzureClient("test-key", "", srv.URL, 5*time.Second)
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

func TestAzureTranslateCachesSameText(t *testing.T) {
	var calls atomic.Int32
	srv := mockAzure(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewAzureClient("test-key", "", srv.URL, 5*time.Second)
	for i := 0; i < 3; i++ {
		if _, err := c.Translate(context.Background(), "Hello"); err != nil {
			t.Fatalf("Translate 返回错误: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP 请求次数 = %d，想要 1（同一条只翻一次）", calls.Load())
	}
}

func TestAzureTranslateMissingKey(t *testing.T) {
	c := NewAzureClient("", "", "http://127.0.0.1:1", 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("未填 Key 应返回错误")
	}
}

func TestAzureTranslateServerError(t *testing.T) {
	var calls atomic.Int32
	srv := mockAzure(t, &calls, "", http.StatusForbidden)
	defer srv.Close()

	c := NewAzureClient("test-key", "", srv.URL, 5*time.Second)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("HTTP 403 应返回错误")
	}
}

func TestAzureTranslateTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewAzureClient("test-key", "", srv.URL, 100*time.Millisecond)
	if _, err := c.Translate(context.Background(), "Hello"); err == nil {
		t.Fatal("超时应返回错误")
	}
}

func TestAzureCheck(t *testing.T) {
	var calls atomic.Int32
	srv := mockAzure(t, &calls, "你好", http.StatusOK)
	defer srv.Close()

	c := NewAzureClient("test-key", "", srv.URL, 5*time.Second)
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("Check 返回错误: %v", err)
	}
	if err := NewAzureClient("", "", srv.URL, 5*time.Second).Check(context.Background()); err == nil {
		t.Fatal("未填 Key 时 Check 应返回错误")
	}
}

// TestAzureTranslateUsesEnvProxy：翻译走外网 API，必须经统一代理选择，
// 否则只改系统代理（v2rayN）的桌面启动方式下请求直连慢路。
// Linux 上裸客户端的 DefaultTransport 本来就认环境变量，行为测不出差别，
// 真正的差别在 Windows 系统代理，所以这里钉接线：传输层的 Proxy 必须是
// 统一选择函数 httpx.ProxyForRequest。
func TestAzureTranslateUsesEnvProxy(t *testing.T) {
	c := NewAzureClient("test-key", "", "http://translate-test.invalid", 5*time.Second)
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("Azure 客户端没有显式传输层（Transport = %v）", c.http.Transport)
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(httpx.ProxyForRequest).Pointer() {
		t.Fatalf("Azure 客户端没走统一代理选择函数 httpx.ProxyForRequest")
	}

	// 行为烟囱：经环境变量代理拿到译文。
	var viaProxy atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"translations": []map[string]any{{"text": "你好，世界", "to": "zh-Hans"}}},
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

func setTranslateTestProxyEnv(t *testing.T, proxyURL string) {
	t.Helper()
	t.Setenv("HTTP_PROXY", proxyURL)
	t.Setenv("http_proxy", proxyURL)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
}
