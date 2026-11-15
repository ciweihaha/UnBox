package mpvplugin

import "testing"

func TestInstallCommandPackageManagerSelection(t *testing.T) {
	if got := commandFor("apt"); got != "sudo apt install mpv" {
		t.Fatalf("apt = %q", got)
	}
	if got := commandFor("apt-get"); got != "sudo apt install mpv" {
		t.Fatalf("apt-get = %q", got)
	}
	if got := commandFor("dnf"); got != "sudo dnf install mpv" {
		t.Fatalf("dnf = %q", got)
	}
	if got := commandFor("pacman"); got != "sudo pacman -S mpv" {
		t.Fatalf("pacman = %q", got)
	}
}

// installCommandForOS 是生产路径（Status/Install 都走它），必须复用
// commandFor 的映射，而不是各写一份——否则上面那个测试测的是死代码。
func TestInstallCommandForOSUsesCommandFor(t *testing.T) {
	if got := installCommandForOS("linux", ""); got == "" {
		t.Fatal("linux 应返回一条安装命令")
	} else if got != commandFor("apt-get") && got != commandFor("apt") && got != commandFor("dnf") && got != commandFor("pacman") {
		t.Fatalf("linux 安装命令应来自 commandFor 映射: %q", got)
	}
	if got := installCommandForOS("darwin", ""); got != "brew install mpv" {
		t.Fatalf("darwin = %q", got)
	}
	if got := installCommandForOS("windows", ""); got != "" {
		t.Fatalf("windows 应返回空串（走下载安装）: %q", got)
	}
}
