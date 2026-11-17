//go:build !windows

package livesub

import "os/exec"

// hideConsole 在非 Windows 上是空操作。
func hideConsole(cmd *exec.Cmd) {}
