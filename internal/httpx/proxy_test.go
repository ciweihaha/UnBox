package httpx

import (
	"net/http"
	"net/url"
	"testing"
)

// TestParseProxyServer：Windows 注册表 ProxyServer 的两种写法都要认。
func TestParseProxyServer(t *testing.T) {
	mustURL := func(t *testing.T, raw string) *url.URL {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", raw, err)
		}
		return u
	}
	cases := []struct {
		name  string
		value string
		http  *url.URL
		https *url.URL
	}{
		{"单地址全协议共用", "127.0.0.1:10809", mustURL(t, "http://127.0.0.1:10809"), mustURL(t, "http://127.0.0.1:10809")},
		{"单地址带 scheme", "http://127.0.0.1:10809", mustURL(t, "http://127.0.0.1:10809"), mustURL(t, "http://127.0.0.1:10809")},
		{"分协议写", "http=127.0.0.1:10809;https=127.0.0.1:10810", mustURL(t, "http://127.0.0.1:10809"), mustURL(t, "http://127.0.0.1:10810")},
		{"只有 http", "http=127.0.0.1:10809", mustURL(t, "http://127.0.0.1:10809"), nil},
		{"只有 socks 不影响 http", "socks=127.0.0.1:10808", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpProxy, httpsProxy, err := parseProxyServer(tc.value)
			if err != nil {
				t.Fatalf("parseProxyServer(%q) 报错: %v", tc.value, err)
			}
			if (httpProxy == nil) != (tc.http == nil) || (httpProxy != nil && httpProxy.String() != tc.http.String()) {
				t.Fatalf("http 代理 = %v，想要 %v", httpProxy, tc.http)
			}
			if (httpsProxy == nil) != (tc.https == nil) || (httpsProxy != nil && httpsProxy.String() != tc.https.String()) {
				t.Fatalf("https 代理 = %v，想要 %v", httpsProxy, tc.https)
			}
		})
	}
}

func TestParseProxyServerBad(t *testing.T) {
	for _, value := range []string{"", "   ", "http=", "://", "http://[::1"} {
		if _, _, err := parseProxyServer(value); err == nil {
			t.Fatalf("parseProxyServer(%q) 应该报错", value)
		}
	}
}

func TestParseProxyOverride(t *testing.T) {
	got := parseProxyOverride(" <local> ; 192.168.1.0/24 ;*.example.com;")
	if len(got) != 3 || got[0] != "<local>" || got[1] != "192.168.1.0/24" || got[2] != "*.example.com" {
		t.Fatalf("parseProxyOverride 结果 = %v", got)
	}
	if len(parseProxyOverride("")) != 0 {
		t.Fatalf("空 ProxyOverride 应该返回空列表")
	}
}

func TestProxyBypassed(t *testing.T) {
	cases := []struct {
		host   string
		bypass []string
		want   bool
	}{
		{"api.example.com", []string{"api.example.com"}, true},
		{"sub.api.example.com", []string{"*.api.example.com"}, true},
		{"api.example.com", []string{"*.api.example.com"}, false},
		{"intranet", []string{"<local>"}, true},
		{"intranet.example.com", []string{"<local>"}, false},
		{"example.com", []string{"other.com"}, false},
		{"example.com", nil, false},
	}
	for _, tc := range cases {
		if got := proxyBypassed(tc.host, tc.bypass); got != tc.want {
			t.Fatalf("proxyBypassed(%q, %v) = %v，想要 %v", tc.host, tc.bypass, got, tc.want)
		}
	}
}

// TestProxyForRequestSystemFallback：环境变量没配时代理走系统设置，
// 环境变量配了时环境变量优先；本机回环地址永远直连。
func TestProxyForRequestSystemFallback(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("https_proxy", "")
	t.Setenv("all_proxy", "")
	t.Setenv("no_proxy", "")

	old := systemProxyForRequest
	t.Cleanup(func() { systemProxyForRequest = old })
	sysProxy, _ := url.Parse("http://127.0.0.1:10809")
	systemProxyForRequest = func(req *http.Request) (*url.URL, error) {
		return sysProxy, nil
	}

	req, _ := http.NewRequest(http.MethodGet, "https://live.ciweihaha.win/vod.json", nil)
	got, err := ProxyForRequest(req)
	if err != nil {
		t.Fatalf("ProxyForRequest 报错: %v", err)
	}
	if got == nil || got.String() != sysProxy.String() {
		t.Fatalf("系统代理没生效，得到 %v，想要 %v", got, sysProxy)
	}

	// 回环地址不进代理。
	loopReq, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/x", nil)
	got, err = ProxyForRequest(loopReq)
	if err != nil || got != nil {
		t.Fatalf("回环地址应该直连，得到 %v / %v", got, err)
	}

	// 环境变量优先于系统设置。
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:18080")
	envProxy, _ := url.Parse("http://127.0.0.1:18080")
	got, err = ProxyForRequest(req)
	if err != nil {
		t.Fatalf("ProxyForRequest 报错: %v", err)
	}
	if got == nil || got.String() != envProxy.String() {
		t.Fatalf("环境变量应该优先，得到 %v，想要 %v", got, envProxy)
	}
}
