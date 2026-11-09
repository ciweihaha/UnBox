// Package mpvplugin 管理外部 mpv 的探测与安装。
package mpvplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvproc"
)

const (
	windowsURL  = "https://github.com/mpv-distributions/mpv-windows-setup/releases/download/0.41.0/mpv-setup-x86_64-0.41.0.exe"
	maxDownload = int64(80 << 20)
)

// windowsSHA 是安装包的校验和（var 而非 const，供测试注入假安装包的哈希）。
var windowsSHA = "1b32d5eb7e713ecc5853c18107daffac652e29474dfd517a4ddb792dc45e40fc"

// windowsDownloadTimeout 是 Windows 上下载 mpv 安装包的总时限。调用方传的
// 是 context.Background()（InstallMPV 无取消入口），没有它，下载中途停滞
// （服务器挂死、中间设备黑洞）会让安装界面无限挂起。15 分钟对 80MB 安装包
// 足够宽松（1Mbps 也只需约 11 分钟），只防「永远等不到字节」。
var windowsDownloadTimeout = 15 * time.Minute

// executablePath 是 os.Executable 的可注入替身，供测试覆盖「应用内嵌 mpv」路径。
var executablePath = os.Executable

type Status struct {
	Available      bool
	Path           string
	InstallMode    string
	InstallCommand string
}
type InstallResult struct {
	Installed bool
	Message   string
}

type Manager struct {
	goos     string
	root     string
	lookPath func(string) (string, error)
	client   *http.Client
	run      func(context.Context, string, ...string) error
	// output 取子进程的标准输出，供 mpv --version 预检使用（run 只回传错误）。
	output func(context.Context, string, ...string) ([]byte, error)
}

func New(goos, root string) *Manager {
	if goos == "" {
		goos = runtime.GOOS
	}
	if root == "" {
		root, _ = os.UserConfigDir()
	}
	return newManager(goos, root, exec.LookPath)
}

func newManager(goos, root string, lookPath func(string) (string, error)) *Manager {
	return &Manager{goos: goos, root: root, lookPath: lookPath, client: http.DefaultClient, run: func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}, output: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).Output()
	}}
}

func (m *Manager) Status() Status {
	if path := m.bundledPath(); path != "" {
		return Status{Available: true, Path: path}
	}
	if path := m.pluginPath(); path != "" {
		return Status{Available: true, Path: path}
	}
	if path, err := m.lookPath(exeForOS(m.goos)); err == nil {
		return Status{Available: true, Path: path}
	}
	s := Status{InstallMode: "manual"}
	s.InstallCommand = installCommandForOS(m.goos, m.root)
	if s.InstallCommand != "" {
		s.InstallMode = "command"
	}
	if m.goos == "windows" {
		s.InstallMode = "download"
	}
	return s
}

func (m *Manager) Install(ctx context.Context) (InstallResult, error) {
	switch m.goos {
	case "linux", "darwin":
		command := installCommandForOS(m.goos, m.root)
		if command == "" {
			return InstallResult{}, errors.New("当前系统没有可用的 mpv 安装命令")
		}
		return InstallResult{Message: "请在终端执行: " + command}, nil
	case "windows":
		return m.installWindows(ctx)
	default:
		return InstallResult{}, errors.New("当前系统暂不支持自动安装 mpv")
	}
}

// NewPlayer 在创建播放器前先做一次 mpv 预检：确认可执行文件真的能跑起来，
// 且版本不低于下限。坏掉的 mpv 走到 IPC 才失败，只会留下一句无从下手的
// 「找不到管道」，预检能在这里就给出可操作的原因。
func (m *Manager) NewPlayer() (player.Player, error) {
	s := m.Status()
	if !s.Available {
		return nil, ErrUnavailable()
	}
	if err := m.checkVersion(context.Background(), s.Path); err != nil {
		return nil, err
	}
	return mpvproc.New(s.Path)
}

func ErrUnavailable() error { return errors.New("未找到 mpv 可执行文件") }

// isDownloadTimeout 判断下载错误是否由下载时限触发（超时或上层取消）。
func isDownloadTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func (m *Manager) installWindows(ctx context.Context) (InstallResult, error) {
	dlCtx, cancel := context.WithTimeout(ctx, windowsDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, windowsURL, nil)
	if err != nil {
		return InstallResult{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		if isDownloadTimeout(err) {
			return InstallResult{}, errors.New("下载 mpv 安装包超时，请检查网络后重试")
		}
		return InstallResult{}, fmt.Errorf("下载 mpv 安装包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return InstallResult{}, fmt.Errorf("下载 mpv 安装包失败: HTTP %d", resp.StatusCode)
	}
	dir := filepath.Join(m.root, "unbox", "plugins", "mpv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("创建 mpv 插件目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "mpv-*.exe")
	if err != nil {
		return InstallResult{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	n, err := io.CopyN(io.MultiWriter(tmp, hash), resp.Body, maxDownload+1)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = tmp.Close()
		if isDownloadTimeout(err) {
			return InstallResult{}, errors.New("下载 mpv 安装包超时，请检查网络后重试")
		}
		return InstallResult{}, fmt.Errorf("保存 mpv 安装包失败: %w", err)
	}
	if n > maxDownload {
		_ = tmp.Close()
		return InstallResult{}, errors.New("mpv 安装包过大")
	}
	if err := tmp.Close(); err != nil {
		return InstallResult{}, err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, windowsSHA) {
		return InstallResult{}, errors.New("mpv 安装包校验失败")
	}
	if err := m.run(ctx, tmpName, "/VERYSILENT", "/CURRENTUSER", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR="+dir); err != nil {
		return InstallResult{}, fmt.Errorf("运行 mpv 安装程序失败: %w", err)
	}
	// 安装程序退出码为 0 不代表 mpv 真落盘（杀软隔离、磁盘满都可能让安装
	// 「成功」却无文件）；按 Status() 找插件的同一路径校验，缺失即报错，
	// 否则用户会看到「安装完成」却仍播不了。
	installed := filepath.Join(dir, exeForOS(m.goos))
	if _, err := os.Stat(installed); err != nil {
		return InstallResult{}, fmt.Errorf("mpv 安装完成但未找到 %s，请重新安装", installed)
	}
	return InstallResult{Installed: true, Message: "mpv 插件安装完成"}, nil
}

func (m *Manager) pluginPath() string {
	path := filepath.Join(m.root, "unbox", "plugins", "mpv", exeForOS(m.goos))
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// bundledPath 返回随应用分发的 mpv 路径（Windows NSIS 内嵌在 exe 旁 mpv/ 目录）。
// 非 Windows 或未找到时返回 ""。
func (m *Manager) bundledPath() string {
	if m.goos != "windows" {
		return ""
	}
	exe, err := executablePath()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), "mpv", exeForOS(m.goos))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func exeForOS(goos string) string {
	if goos == "windows" {
		return "mpv.exe"
	}
	return "mpv"
}
func installCommandForOS(goos, _ string) string {
	switch goos {
	case "darwin":
		return "brew install mpv"
	case "linux":
		for _, pair := range []struct{ name, command string }{{"apt-get", "sudo apt install mpv"}, {"apt", "sudo apt install mpv"}, {"dnf", "sudo dnf install mpv"}, {"pacman", "sudo pacman -S mpv"}} {
			if _, err := exec.LookPath(pair.name); err == nil {
				return pair.command
			}
		}
		return "sudo apt install mpv"
	default:
		return ""
	}
}
func commandFor(name string) string {
	return map[string]string{"apt": "sudo apt install mpv", "dnf": "sudo dnf install mpv", "pacman": "sudo pacman -S mpv"}[name]
}
