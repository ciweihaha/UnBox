package shell

import (
	"fmt"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SetTitleBarColour 把窗口顶部那条原生标题栏改成给定颜色（#RRGGBB），跟用户
// 选的背景一个色系，不再白一条。Windows 下经 DWM 设标题栏颜色，深色同时开
// 沉浸式暗色模式（最小化/最大化/关闭的字形变浅）；非 Windows 平台是空操作。
// 没有 Wails 应用实例时（测试）也是空操作；颜色非法时返回错误。
func (s *ShellService) SetTitleBarColour(colour string) error {
	r, g, b, err := parseTitleBarColour(colour)
	if err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return nil
	}
	win := app.Window.Current()
	if win == nil {
		return nil
	}
	handle := win.NativeWindow()
	if handle == nil {
		return nil
	}
	return applyTitleBarColour(handle, r, g, b, titleBarIsDark(r, g, b))
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
