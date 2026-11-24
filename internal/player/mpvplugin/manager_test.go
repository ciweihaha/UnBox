package mpvplugin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unbox/unbox/internal/httpx"
)

func TestManagerStatusPrefersPluginExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "unbox", "plugins", "mpv", "mpv.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newManager("windows", dir, func(string) (string, error) { return `C:\\system\\mpv.exe`, nil })
	got := m.Status()
	if !got.Available || got.Path != exe {
		t.Fatalf("Status = %+v", got)
	}
}

func TestManagerUnixReturnsInstallCommandWithoutRunningIt(t *testing.T) {
	for _, tc := range []struct{ goos, want string }{{"linux", "sudo apt install mpv"}, {"darwin", "brew install mpv"}} {
		m := newManager(tc.goos, t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
		got := m.Status()
		if got.Available || got.InstallMode != "command" || got.InstallCommand == "" {
			t.Fatalf("%s status = %+v", tc.goos, got)
		}
		if got.InstallCommand != tc.want {
			t.Fatalf("%s command = %q", tc.goos, got.InstallCommand)
		}
	}
}

func TestManagerInstallUnsupportedOSIsManual(t *testing.T) {
	m := newManager("plan9", t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
	if got := m.Status(); got.InstallMode != "manual" {
		t.Fatalf("status = %+v", got)
	}
	if _, err := m.Install(context.Background()); err == nil {
		t.Fatal("unsupported install should fail")
	}
}

func TestManagerStatusPrefersBundledExecutable(t *testing.T) {
	exeDir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(filepath.Join(exeDir, "mpv"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundled := filepath.Join(exeDir, "mpv", "mpv.exe")
	if err := os.WriteFile(bundled, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := executablePath
	executablePath = func() (string, error) { return filepath.Join(exeDir, "unbox.exe"), nil }
	t.Cleanup(func() { executablePath = orig })
	// lookPath 返回一个「系统 mpv」，确保内嵌优先于系统。
	m := newManager("windows", t.TempDir(), func(string) (string, error) { return `C:\system\mpv.exe`, nil })
	got := m.Status()
	if !got.Available || got.Path != bundled {
		t.Fatalf("Status = %+v, want bundled %q", got, bundled)
	}
}

// TestNewManagerClientUsesEnvProxy：Windows 上自动下载 mpv 安装包走外网，
// 必须经统一代理选择，否则只改系统代理（v2rayN）的桌面启动方式下直连慢路。
// Linux 上裸客户端的 DefaultTransport 本来就认环境变量，行为测不出差别，
// 真正的差别在 Windows 系统代理，所以这里钉接线：传输层的 Proxy 必须是
// 统一选择函数 httpx.ProxyForRequest。
func TestNewManagerClientUsesEnvProxy(t *testing.T) {
	m := newManager("windows", t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
	tr, ok := m.client.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("mpv 下载客户端没有显式传输层（Transport = %v）", m.client.Transport)
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(httpx.ProxyForRequest).Pointer() {
		t.Fatalf("mpv 下载客户端没走统一代理选择函数 httpx.ProxyForRequest")
	}
}
