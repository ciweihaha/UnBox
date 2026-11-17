//go:build windows

package livesub

import (
	"os/exec"
	"syscall"
)

// hideConsole 让子进程不在 Windows 上弹出黑窗口
// （UnBox 是 GUI 程序，whisper-server 的控制台窗口会吓到用户）。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
