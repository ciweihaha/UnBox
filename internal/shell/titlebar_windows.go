//go:build windows

package shell

import (
	"log"
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
// 逐项返回 HRESULT（0 为成功）：Win10/Server 上标题栏颜色与边框改色会失败，
// 那是系统限制；但暗色模式必须成功——标题栏颜色失败后要再补一次暗色模式，
// 因为失败的改色调用可能把暗色模式也带回默认，标题栏会变回白色。
func applyTitleBarColour(handle unsafe.Pointer, r, g, b uint8, dark bool) TitleBarColourResult {
	res := TitleBarColourResult{}
	if err := dwmapi.Load(); err != nil {
		res.DarkModeCode = -1
		res.CaptionCode = -1
		res.BorderCode = -1
		return res
	}
	hwnd := windows.HWND(handle)
	if hwnd == 0 {
		return res
	}
	colour := uint32(r) | uint32(g)<<8 | uint32(b)<<16 // COLORREF：0x00BBGGRR
	setAttr := func(attr uint32, value uint32) int32 {
		ret, _, _ := procDwmSetWindowAttribute.Call(
			uintptr(hwnd),
			uintptr(attr),
			uintptr(unsafe.Pointer(&value)),
			unsafe.Sizeof(value),
		)
		return int32(ret) // 返回值是 HRESULT，0 为 S_OK
	}
	var darkMode uint32
	if dark {
		darkMode = 1
	}
	res.DarkModeCode = setAttr(dwmwaUseImmersiveDarkMode, darkMode)
	res.CaptionCode = setAttr(dwmwaCaptionColor, colour)
	res.BorderCode = setAttr(dwmwaBorderColor, colour)
	if res.CaptionCode != 0 || res.BorderCode != 0 {
		// 改色失败时再补一次暗色模式，防标题栏被带回白色。
		res.DarkModeCode = setAttr(dwmwaUseImmersiveDarkMode, darkMode)
	}
	res.DarkModeApplied = res.DarkModeCode == 0
	res.CaptionApplied = res.CaptionCode == 0
	res.BorderApplied = res.BorderCode == 0
	if !res.CaptionApplied || !res.BorderApplied || !res.DarkModeApplied {
		log.Printf("标题栏着色未全生效：暗色模式=%#x 标题栏=%#x 边框=%#x",
			uint32(res.DarkModeCode), uint32(res.CaptionCode), uint32(res.BorderCode))
	}
	return res
}
