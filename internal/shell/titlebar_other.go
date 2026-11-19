//go:build !windows

package shell

import "unsafe"

// applyTitleBarColour 在非 Windows 平台是空操作（原生标题栏由系统主题管）。
func applyTitleBarColour(handle unsafe.Pointer, r, g, b uint8, dark bool) error {
	return nil
}
