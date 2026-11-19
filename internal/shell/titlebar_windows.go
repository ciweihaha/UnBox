//go:build windows

package shell

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// DWM 窗口属性：标题栏/边框颜色只在 Windows 11 上支持；Windows 10 上调用会
// 失败，调用方按尽力而为处理（暗色模式属性本身 Windows 10 1903+ 就支持）。
const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaBorderColor          = 34
	dwmwaCaptionColor         = 35
)

// applyTitleBarColour 经 DWM 把标题栏（含边框）设成给定颜色；dark 为真时同时
// 开沉浸式暗色模式（标题栏按钮字形变浅）。DWM 调用可跨线程，不必切主线程。
// 任一属性失败只记最后的错，不阻断其他属性。
func applyTitleBarColour(handle unsafe.Pointer, r, g, b uint8, dark bool) error {
	if err := dwmapi.Load(); err != nil {
		return fmt.Errorf("加载 dwmapi.dll 失败: %w", err)
	}
	hwnd := windows.HWND(handle)
	if hwnd == 0 {
		return nil
	}
	colour := uint32(r) | uint32(g)<<8 | uint32(b)<<16 // COLORREF：0x00BBGGRR
	var lastErr error
	setAttr := func(attr uint32, value uint32) {
		ret, _, _ := procDwmSetWindowAttribute.Call(
			uintptr(hwnd),
			uintptr(attr),
			uintptr(unsafe.Pointer(&value)),
			unsafe.Sizeof(value),
		)
		if ret != 0 { // 返回值是 HRESULT，0 为 S_OK
			lastErr = fmt.Errorf("DwmSetWindowAttribute(%d) 失败：HRESULT %#x", attr, ret)
		}
	}
	var darkMode uint32
	if dark {
		darkMode = 1
	}
	setAttr(dwmwaUseImmersiveDarkMode, darkMode)
	setAttr(dwmwaCaptionColor, colour)
	setAttr(dwmwaBorderColor, colour)
	return lastErr
}
