//go:build !windows

package shell

import "unsafe"

// applyTitleBarColour 在非 Windows 平台是空操作（原生标题栏由系统主题管）。
// 返回全项已生效：没有要改的东西，也就没有失败项。
func applyTitleBarColour(handle unsafe.Pointer, r, g, b uint8, dark bool) TitleBarColourResult {
	return TitleBarColourResult{DarkModeApplied: true, CaptionApplied: true, BorderApplied: true}
}
