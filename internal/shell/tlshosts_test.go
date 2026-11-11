package shell

import "testing"

// TestNormalizeTLSHostIPv6 是回归测试（第 42 轮）：裸 IPv6 主机（如
// 2001:db8::1）不能按最后一个冒号切端口，否则存成截断的 2001:db8:，
// 与运行时的完整主机永远对不上，证书豁免对 IPv6 源站静默失效——
// 与第 29 轮 httpx.normalizeHost 的同款问题。
func TestNormalizeTLSHostIPv6(t *testing.T) {
	cases := map[string]string{
		"example.com":             "example.com",
		"Example.COM:8443":        "example.com",
		"https://example.com/x":   "example.com",
		"  example.com  ":         "example.com",
		"2001:DB8::1":             "2001:db8::1",
		"[2001:db8::1]:8443":      "2001:db8::1",
		"https://[2001:db8::1]/x": "2001:db8::1",
		"127.0.0.1:8080":          "127.0.0.1",
		"":                        "",
	}
	for in, want := range cases {
		if got := normalizeTLSHost(in); got != want {
			t.Errorf("normalizeTLSHost(%q) = %q，期望 %q", in, got, want)
		}
	}
}
