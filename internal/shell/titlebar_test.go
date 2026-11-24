package shell

import "testing"

// 2026-11-19：窗口顶部的白色标题栏太突兀，改成和用户选的背景一个色系。
// Windows 下经 DWM 把标题栏颜色设成主题背景色（深色同时开沉浸式暗色模式，
// 让最小化/最大化/关闭字形变浅）；非 Windows 平台是空操作。
func TestParseTitleBarColour(t *testing.T) {
	cases := []struct {
		in       string
		r, g, b  uint8
		wantFail bool
	}{
		{"#141414", 0x14, 0x14, 0x14, false},
		{"#E2D5C0", 0xE2, 0xD5, 0xC0, false},
		{"#120324", 0x12, 0x03, 0x24, false},
		{"141414", 0, 0, 0, true},
		{"#12345", 0, 0, 0, true},
		{"#gggggg", 0, 0, 0, true},
		{"", 0, 0, 0, true},
	}
	for _, c := range cases {
		r, g, b, err := parseTitleBarColour(c.in)
		if c.wantFail {
			if err == nil {
				t.Errorf("parseTitleBarColour(%q) 应该失败", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTitleBarColour(%q) 失败: %v", c.in, err)
			continue
		}
		if r != c.r || g != c.g || b != c.b {
			t.Errorf("parseTitleBarColour(%q) = %d,%d,%d，想要 %d,%d,%d", c.in, r, g, b, c.r, c.g, c.b)
		}
	}
}

func TestTitleBarIsDark(t *testing.T) {
	if !titleBarIsDark(0x14, 0x14, 0x14) {
		t.Error("#141414 应判为深色")
	}
	if titleBarIsDark(0xE2, 0xD5, 0xC0) {
		t.Error("#E2D5C0 应判为浅色")
	}
	if !titleBarIsDark(0x0E, 0x0E, 0x14) {
		t.Error("#0e0e14 应判为深色")
	}
}

func TestSetTitleBarColourNoAppIsNoop(t *testing.T) {
	// 测试里没有 Wails 应用实例：不应崩也不应报错，直接空操作。
	svc := NewShellService(nil, nil, nil)
	if _, err := svc.SetTitleBarColour("#141414"); err != nil {
		t.Errorf("没有应用实例时应空操作，得到错误: %v", err)
	}
	if _, err := svc.SetTitleBarColour("不是颜色"); err == nil {
		t.Error("非法颜色应返回错误")
	}
}

func TestSetTitleBarColourResultShape(t *testing.T) {
	// 没有应用实例时返回零值结果、不报错；结果结构供前端判断用。
	svc := NewShellService(nil, nil, nil)
	res, err := svc.SetTitleBarColour("#141414")
	if err != nil {
		t.Fatalf("SetTitleBarColour: %v", err)
	}
	if res.DarkModeApplied || res.CaptionApplied || res.BorderApplied {
		t.Errorf("没有应用实例时不应谎报已生效：%+v", res)
	}
}

// P01：标题栏要带版本号，用户在任务栏/窗口切换里一眼能确认装的是哪版，
// 不用再进「关于」页查。
func TestWindowTitle(t *testing.T) {
	old := appVersion
	defer func() { appVersion = old }()

	appVersion = "0.9.17"
	if got := windowTitle(); got != "UnBox 0.9.17" {
		t.Fatalf("windowTitle() = %q，想要 %q", got, "UnBox 0.9.17")
	}
	// 未注入版本（本地开发默认）也不留尾巴空格。
	appVersion = ""
	if got := windowTitle(); got != "UnBox" {
		t.Fatalf("windowTitle() = %q，想要 %q", got, "UnBox")
	}
	appVersion = "  "
	if got := windowTitle(); got != "UnBox" {
		t.Fatalf("windowTitle() = %q，想要 %q", got, "UnBox")
	}
}
