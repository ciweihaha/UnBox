package shell

import (
	"fmt"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TitleBarColourResult 是标题栏着色的逐项结果：HRESULT 为 0 表示该项成功。
// Windows 10 / Server 不支持改标题栏颜色（DWMWA_CAPTION_COLOR 只要 Win11），
// 此时 CaptionApplied 为假，但沉浸式暗色模式（DarkModeApplied）仍应生效，
// 标题栏至少是深色而不是白色。前端据此判断颜色是否真生效、失败时给用户
// 一句明白话；云电脑实测也按这三项定位是哪一级 DWM 调用失败。
type TitleBarColourResult struct {
	DarkModeApplied bool
	CaptionApplied  bool
	BorderApplied   bool
	DarkModeCode    int32
	CaptionCode     int32
	BorderCode      int32
}

// SetTitleBarColour 把窗口顶部那条原生标题栏改成给定颜色（#RRGGBB），跟用户
// 选的背景一个色系，不再白一条。Windows 下经 DWM 设标题栏颜色，深色同时开
// 沉浸式暗色模式（最小化/最大化/关闭的字形变浅）；非 Windows 平台是空操作。
// 没有 Wails 应用实例时（测试）也是空操作；颜色非法时返回错误。
// DWM 单项失败不返回 error，结果里逐项给 HRESULT：Win10/Server 上标题栏
// 颜色与边框改色本来就会失败（只要 Win11），那两项失败不等于整体失败。
func (s *ShellService) SetTitleBarColour(colour string) (TitleBarColourResult, error) {
	r, g, b, err := parseTitleBarColour(colour)
	if err != nil {
		return TitleBarColourResult{}, err
	}
	app := application.Get()
	if app == nil {
		return TitleBarColourResult{}, nil
	}
	win := app.Window.Current()
	if win == nil {
		return TitleBarColourResult{}, nil
	}
	handle := win.NativeWindow()
	if handle == nil {
		return TitleBarColourResult{}, nil
	}
	return applyTitleBarColour(handle, r, g, b, titleBarIsDark(r, g, b)), nil
}

// parseTitleBarColour 解析 #RRGGBB 颜色；格式不对返回错误。
func parseTitleBarColour(s string) (r, g, b uint8, err error) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, fmt.Errorf("标题栏颜色须为 #RRGGBB，得到 %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("标题栏颜色须为 #RRGGBB，得到 %q", s)
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), nil
}

// titleBarIsDark 按相对亮度判断颜色是否深色（深色标题栏要开暗色模式字形）。
func titleBarIsDark(r, g, b uint8) bool {
	lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return lum < 128
}
