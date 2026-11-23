//go:build windows

package httpx

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// readSystemProxyConfig 读 Windows 系统代理设置
// （HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings：
// ProxyEnable + ProxyServer + ProxyOverride，v2rayN 的「系统代理」
// 模式就是改这三个值）。没开系统代理时返回未启用。
func readSystemProxyConfig() *systemProxyConfig {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	enabled, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return nil
	}
	httpProxy, httpsProxy, err := parseProxyServer(server)
	if err != nil {
		return nil
	}
	cfg := &systemProxyConfig{enabled: true, http: httpProxy, https: httpsProxy}
	if override, _, err := k.GetStringValue("ProxyOverride"); err == nil {
		cfg.bypass = parseProxyOverride(override)
	}
	return cfg
}
