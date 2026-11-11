package live

import (
	"strings"
	"testing"
)

// TestParseM3UQuotedCommaInAttr 是回归测试（第 22 轮）：属性值里的逗号
// （如 group-title="综艺,电影"）不能被当成标题分隔符，否则频道名错乱。
func TestParseM3UQuotedCommaInAttr(t *testing.T) {
	raw := []byte("#EXTM3U\n" +
		"#EXTINF:-1 tvg-id=\"cctv1\" group-title=\"综艺,电影\",CCTV-1 综合\n" +
		"http://x/live/cctv1.m3u8\n")
	got := ParseM3U(raw)
	if len(got) != 1 {
		t.Fatalf("ParseM3U = %#v, want 1 条", got)
	}
	if got[0].Name != "CCTV-1 综合" || got[0].Group != "综艺,电影" || got[0].ID != "cctv1" {
		t.Fatalf("ParseM3U = %#v, want Name=CCTV-1 综合 Group=综艺,电影 ID=cctv1", got[0])
	}
}

// TestParseM3UAttrSubstring 是回归测试（第 22 轮）：属性名不能在别的
// 属性名或属性值里做子串命中——x-tvg-id 不是 tvg-id，logo URL 查询串里
// 的 group-title= 也不是 group-title 属性。
func TestParseM3UAttrSubstring(t *testing.T) {
	raw := []byte("#EXTM3U\n" +
		"#EXTINF:-1 x-tvg-id=\"wrong\" tvg-id=\"right\" tvg-logo=\"http://cdn/x.png?group-title=news\",频道A\n" +
		"http://x/a\n")
	got := ParseM3U(raw)
	if len(got) != 1 {
		t.Fatalf("ParseM3U = %#v, want 1 条", got)
	}
	if got[0].ID != "right" {
		t.Fatalf("ID = %q, want right（命中了 x-tvg-id）", got[0].ID)
	}
	if got[0].Group != "" {
		t.Fatalf("Group = %q, want 空（命中了 logo URL 里的 group-title=）", got[0].Group)
	}
	if got[0].Logo != "http://cdn/x.png?group-title=news" {
		t.Fatalf("Logo = %q, want 完整 URL", got[0].Logo)
	}
}

// TestParseM3UOver1MBLine 是回归测试（第 22 轮）：单行超过 1MB 时，
// Scanner 会直接停掉、静默丢掉该行之后的所有频道；解析必须不受行长影响。
func TestParseM3UOver1MBLine(t *testing.T) {
	junk := strings.Repeat("x", 2<<20)
	raw := []byte("#EXTM3U\n" +
		"#EXTINF:-1,首频道\n" +
		"http://x/first\n" +
		junk + "\n" +
		"#EXTINF:-1,尾频道\n" +
		"http://x/last\n")
	got := ParseM3U(raw)
	if len(got) != 2 || got[1].Name != "尾频道" {
		t.Fatalf("ParseM3U 得 %d 条，want 2 条且第二条为尾频道", len(got))
	}
}

// TestParseTXTOver1MBLine 同上：TXT 里一行超 1MB 也不应丢掉后面的行。
func TestParseTXTOver1MBLine(t *testing.T) {
	junk := strings.Repeat("x", 2<<20)
	raw := []byte("首频道,http://x/first\n" + junk + "\n尾频道,http://x/last\n")
	got := ParseTXT(raw)
	if len(got) != 2 || got[1].Name != "尾频道" {
		t.Fatalf("ParseTXT 得 %d 条，want 2 条且第二条为尾频道", len(got))
	}
}
