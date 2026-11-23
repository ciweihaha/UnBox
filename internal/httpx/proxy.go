package httpx

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// systemProxyConfig 是从操作系统代理设置读到的配置
// （Windows：HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings）。
type systemProxyConfig struct {
	enabled bool
	http    *url.URL // http:// 代理（没分协议时也充当 https 代理）
	https   *url.URL
	bypass  []string // 不走代理的主机规则，如 <local>、*.example.com
}

// systemProxyForRequest 由平台相关文件提供默认实现；测试可替换整个函数。
var systemProxyForRequest = defaultSystemProxyForRequest

// ProxyForRequest 是全应用统一的代理选择函数。
//
// 桌面双击启动的应用没有 HTTP_PROXY/HTTPS_PROXY 环境变量，Go 的
// ProxyFromEnvironment 读不到本机代理设置（v2rayN 这类本地代理只改
// 系统代理）。2026-11-22 笔记本实测：直连到 Cloudflare 只有约 20B/s，
// 走本机 127.0.0.1:10809 代理 2.6 秒就通；点播搜索与直播拉流全走直连，
// 单站 12 秒预算内拿不到结果就 0 条（D1）。所以环境变量没配时再读
// 一次系统代理设置；环境变量配了时环境变量优先。
// 本机回环地址永远直连，不进代理。
//
// 注意：标准库的 http.ProxyFromEnvironment 在进程内只读一次环境变量
// （sync.Once 缓存），这里改用 x/net/http/httpproxy 每次现读，与标准库
// 同一套解析（含 NO_PROXY 与回环直连），行为一致且可测试。
func ProxyForRequest(req *http.Request) (*url.URL, error) {
	if req != nil && isLoopbackHost(req.URL.Hostname()) {
		return nil, nil
	}
	if proxy, err := httpproxy.FromEnvironment().ProxyFunc()(req.URL); proxy != nil || err != nil {
		return proxy, err
	}
	return systemProxyForRequest(req)
}

// defaultSystemProxyForRequest 按系统代理设置给请求选代理。
func defaultSystemProxyForRequest(req *http.Request) (*url.URL, error) {
	cfg := readSystemProxyConfig()
	if cfg == nil || !cfg.enabled {
		return nil, nil
	}
	if proxyBypassed(req.URL.Hostname(), cfg.bypass) {
		return nil, nil
	}
	if strings.EqualFold(req.URL.Scheme, "https") && cfg.https != nil {
		return cfg.https, nil
	}
	return cfg.http, nil
}

// parseProxyServer 解析 Windows 注册表 ProxyServer 的写法：
// "127.0.0.1:10809"（全协议共用）或
// "http=127.0.0.1:10809;https=127.0.0.1:10809;socks=127.0.0.1:10808"。
// 没写 scheme 的地址按 http:// 补；socks 只影响 socks，不参与 http/https。
func parseProxyServer(value string) (httpProxy, httpsProxy *url.URL, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil, fmt.Errorf("代理地址为空")
	}
	// 分协议写法的每一段是 "协议=地址"。
	if strings.Contains(value, "=") {
		for _, part := range strings.Split(value, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			scheme, addr, ok := strings.Cut(part, "=")
			if !ok || strings.TrimSpace(addr) == "" {
				return nil, nil, fmt.Errorf("代理地址格式错误: %q", value)
			}
			u, perr := parseProxyAddr(strings.TrimSpace(addr))
			if perr != nil {
				return nil, nil, perr
			}
			switch strings.ToLower(strings.TrimSpace(scheme)) {
			case "http":
				httpProxy = u
			case "https":
				httpsProxy = u
			}
		}
		if httpProxy == nil && httpsProxy == nil {
			// 只有 socks 之类与 http/https 无关的项：没有 http 代理，
			// 不是格式错误，调用方按直连处理。
			return nil, nil, nil
		}
		return httpProxy, httpsProxy, nil
	}
	u, err := parseProxyAddr(value)
	if err != nil {
		return nil, nil, err
	}
	return u, u, nil
}

// parseProxyAddr 把 "host:port" 或带 scheme 的地址解析成 URL。
func parseProxyAddr(addr string) (*url.URL, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("代理地址为空")
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("代理地址格式错误: %q", addr)
	}
	return u, nil
}

// parseProxyOverride 解析 ProxyOverride（分号分隔）：
// "<local>"、"192.168.1.0/24"、"*.example.com" 这类写法原样保留。
func parseProxyOverride(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ";") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// proxyBypassed 判断主机是否在不走代理的名单里：
// "<local>" 只放行不带点的主机；"*.example.com" 放行子域；其余精确匹配。
func proxyBypassed(host string, bypass []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, rule := range bypass {
		rule = strings.ToLower(strings.TrimSpace(rule))
		switch {
		case rule == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		case strings.HasPrefix(rule, "*."):
			if strings.HasSuffix(host, rule[1:]) {
				return true
			}
		case rule != "" && host == rule:
			return true
		}
	}
	return false
}

// isLoopbackHost 判断主机是不是本机回环地址。
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
