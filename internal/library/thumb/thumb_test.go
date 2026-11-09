package thumb

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeMPV(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-mpv.sh")
	body := `#!/bin/sh
for i in "$@"; do case "$i" in --vo-image-outdir=*) out="${i#--vo-image-outdir=}";; esac; done
mkdir -p "$out"
printf '\xff\xd8\xff\xd9' > "$out/00000001.jpg"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestGenerateWritesCache(t *testing.T) {
	g := &MpvGenerator{mpvPath: fakeMPV(t), timeout: 5 * time.Second}
	cache := filepath.Join(t.TempDir(), "k.jpg")
	if err := g.Generate("/v/a.mkv", cache); err != nil {
		t.Fatalf("Generate = %v", err)
	}
	b, err := os.ReadFile(cache)
	if err != nil || len(b) == 0 {
		t.Fatalf("cache 未写入或为空: %v", err)
	}
}

// recordingFakeMPV 返回一个假 mpv：把全部参数写进 $UNBOX_TEST_ARGS，再像
// fakeMPV 一样产出一张 jpg。
func recordingFakeMPV(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-mpv.sh")
	body := `#!/bin/sh
printf '%s\n' "$@" > "$UNBOX_TEST_ARGS"
for i in "$@"; do case "$i" in --vo-image-outdir=*) out="${i#--vo-image-outdir=}";; esac; done
mkdir -p "$out"
printf '\xff\xd8\xff\xd9' > "$out/00000001.jpg"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// TestGenerateKeepsOldCacheOnFailure 抓帧失败时旧海报必须保留：不能先删目标再跑 mpv。
func TestGenerateKeepsOldCacheOnFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-mpv.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	g := &MpvGenerator{mpvPath: script, timeout: 5 * time.Second}
	cache := filepath.Join(dir, "k.jpg")
	if err := os.WriteFile(cache, []byte("old-poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate("/v/a.mkv", cache); err == nil {
		t.Fatal("假 mpv 失败时 Generate 应报错")
	}
	b, err := os.ReadFile(cache)
	if err != nil || string(b) != "old-poster" {
		t.Fatalf("失败后旧海报应保留，got %q, %v", b, err)
	}
}

// TestGenerateReplacesOldCacheOnSuccess 抓帧成功时旧海报应被新帧替换。
func TestGenerateReplacesOldCacheOnSuccess(t *testing.T) {
	g := &MpvGenerator{mpvPath: fakeMPV(t), timeout: 5 * time.Second}
	cache := filepath.Join(t.TempDir(), "k.jpg")
	if err := os.WriteFile(cache, []byte("old-poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate("/v/a.mkv", cache); err != nil {
		t.Fatalf("Generate = %v", err)
	}
	b, err := os.ReadFile(cache)
	if err != nil || len(b) == 0 || string(b) == "old-poster" {
		t.Fatalf("成功后缓存应为新帧，got %q, %v", b, err)
	}
}

// TestGeneratePassesAbsoluteVideoPath 传给 mpv 的视频路径必须是绝对路径：
// 相对路径会被 mpv 按工作目录解析，横杠开头的文件名还会被当成选项。
func TestGeneratePassesAbsoluteVideoPath(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	g := &MpvGenerator{mpvPath: recordingFakeMPV(t), timeout: 5 * time.Second}
	t.Setenv("UNBOX_TEST_ARGS", argsFile)
	cache := filepath.Join(t.TempDir(), "k.jpg")
	if err := g.Generate("-weird.mkv", cache); err != nil {
		t.Fatalf("Generate = %v", err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	last := args[len(args)-1]
	if !filepath.IsAbs(last) || strings.HasPrefix(last, "-") {
		t.Fatalf("传给 mpv 的视频路径应为绝对路径，got %q（args=%q）", last, args)
	}
}

func TestGenerateNoMpv(t *testing.T) {
	g := &MpvGenerator{mpvPath: "", timeout: 5 * time.Second}
	if err := g.Generate("/v/a.mkv", "/tmp/x.jpg"); !errors.Is(err, ErrNoMpv) {
		t.Fatalf("want ErrNoMpv, got %v", err)
	}
}
