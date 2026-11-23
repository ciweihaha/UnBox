//go:build !windows

package httpx

// readSystemProxyConfig 在非 Windows 平台上不读系统代理设置，
// 只用环境变量（ProxyForRequest 里的 ProxyFromEnvironment）。
func readSystemProxyConfig() *systemProxyConfig {
	return nil
}
