package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// oversizeScript 拼一段超过 4MB 的合法 JS：超限的脚本现在会被静默截断，
// 丢给 goja 只报难以定位的语法错误，而不是诚实的超限错误。
func oversizeScript(t *testing.T) string {
	t.Helper()
	// 4MB 上限 + 1 字节：头部是合法语句，尾部用块注释垫到超限。
	return "var __oversize_probe = 1;\n/*" + strings.Repeat("x", 4<<20) + "*/\n"
}

func exactLimitScript() string {
	// 精确拼到 4MB 且注释闭合：合法脚本、应正常加载。
	prefix := "var __limit_probe = 1;\n/*"
	suffix := "*/"
	return prefix + strings.Repeat("x", (4<<20)-len(prefix)-len(suffix)) + suffix
}

// TestLoadFromURLRejectsOversizeScript 是回归测试（第 23 轮）：
// 超 4MB 的爬虫脚本不得静默截断，必须报明确的超限错误。
func TestLoadFromURLRejectsOversizeScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, oversizeScript(t))
	}))
	defer srv.Close()

	err := New().LoadFromURL(context.Background(), srv.URL+"/spider.js")
	if err == nil {
		t.Fatal("超限脚本应报错")
	}
	if !strings.Contains(err.Error(), "超过") {
		t.Fatalf("错误应说明脚本超限，实际: %v", err)
	}
	if strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("不应再暴露截断导致的 goja 语法错误，实际: %v", err)
	}
}

// TestLoadFromURLAcceptsExactLimitScript 是边界测试（第 23 轮）：
// 恰好 4MB 的脚本应正常加载，不被超限逻辑误杀。
func TestLoadFromURLAcceptsExactLimitScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, exactLimitScript())
	}))
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/spider.js"); err != nil {
		t.Fatalf("恰好 4MB 的脚本应正常加载，实际: %v", err)
	}
}

// TestFetchModuleRejectsOversizeScript 是回归测试（第 23 轮）：
// ESM 模块下载与主脚本同一上限，超限同样不得静默截断。
func TestFetchModuleRejectsOversizeScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, oversizeScript(t))
	}))
	defer srv.Close()

	_, err := New().fetchModule(context.Background(), srv.URL+"/mod.js")
	if err == nil {
		t.Fatal("超限模块应报错")
	}
	if !strings.Contains(err.Error(), "超过") {
		t.Fatalf("错误应说明模块超限，实际: %v", err)
	}
}
