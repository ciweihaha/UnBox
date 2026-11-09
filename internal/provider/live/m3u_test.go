package live

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseM3UExtractsAttributes(t *testing.T) {
	raw := []byte("#EXTM3U\n" +
		"#EXTINF:-1 tvg-id=\"cctv1\" tvg-logo=\"http://x/1.png\" group-title=\"央视\",CCTV-1 综合\n" +
		"http://x/live/cctv1.m3u8\n" +
		"#EXTINF:-1 group-title=\"卫视\",湖南卫视\n" +
		"http://x/live/hunan.ts\n")
	got := ParseM3U(raw)
	want := []Entry{
		{Name: "CCTV-1 综合", URL: "http://x/live/cctv1.m3u8", Logo: "http://x/1.png", Group: "央视", ID: "cctv1"},
		{Name: "湖南卫视", URL: "http://x/live/hunan.ts", Group: "卫视"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseM3U = %#v, want %#v", got, want)
	}
}

func TestParseM3UToleratesBOMCRLFAndJunk(t *testing.T) {
	raw := []byte("\xef\xbb\xbf#EXTM3U\r\n" +
		"#EXTINF:-1,频道A\r\n" +
		"http://x/a\r\n" +
		"\r\n" +
		"# 注释行\r\n" +
		"http://x/orphan-without-extinf\r\n") // 无 #EXTINF 前导的 URL 应被跳过
	got := ParseM3U(raw)
	if len(got) != 1 || got[0].Name != "频道A" {
		t.Fatalf("ParseM3U = %#v, want 仅 1 条频道A", got)
	}
}

func TestParseM3UMissingURLDropsEntry(t *testing.T) {
	raw := []byte("#EXTM3U\n#EXTINF:-1,只有名字没有 URL\n#EXTINF:-1,正常\nhttp://x/ok\n")
	got := ParseM3U(raw)
	if len(got) != 1 || got[0].URL != "http://x/ok" {
		t.Fatalf("ParseM3U = %#v, want 1 条", got)
	}
}

// TestParseM3ULongLine 单行超过 bufio.Scanner 默认 64KB 上限时，解析不应
// 静默丢掉该行之后的所有频道（超长行常见于带巨大 logo 参数的 #EXTINF）。
func TestParseM3ULongLine(t *testing.T) {
	longLogo := strings.Repeat("x", 100<<10)
	raw := []byte("#EXTM3U\n" +
		"#EXTINF:-1 tvg-logo=\"" + longLogo + "\",超长行频道\n" +
		"http://x/long\n" +
		"#EXTINF:-1,正常频道\n" +
		"http://x/ok\n")
	got := ParseM3U(raw)
	if len(got) != 2 || got[1].Name != "正常频道" {
		t.Fatalf("ParseM3U = %#v, want 2 条且第二条为正常频道", got)
	}
}

// TestParseTXTLongLine 同上：TXT 里一行超长也不应丢掉后面的行。
func TestParseTXTLongLine(t *testing.T) {
	longName := strings.Repeat("x", 100<<10)
	raw := []byte(longName + ",http://x/long\n正常频道,http://x/ok\n")
	got := ParseTXT(raw)
	if len(got) != 2 || got[1].Name != "正常频道" {
		t.Fatalf("ParseTXT = %#v, want 2 条且第二条为正常频道", got)
	}
}

func TestParseTXT(t *testing.T) {
	raw := []byte("频道一,http://x/1\n频道二,http://x/2\n")
	got := ParseTXT(raw)
	if len(got) != 2 || got[1].Name != "频道二" || got[1].URL != "http://x/2" {
		t.Fatalf("ParseTXT = %#v", got)
	}
}

// 把 HTML 网页当 TXT 播放列表时（例如误把导航页填成源地址），含逗号的
// HTML 行不得变成频道。
func TestParseTXTIgnoresHTMLLines(t *testing.T) {
	raw := []byte(`<!DOCTYPE html>
<html>
<head><meta name="viewport" content="width=device-width, initial-scale=1,user-scalable=no" /></head>
<script>gtag('js', new Date());</script>
<body><p>高速免备案,香港日本特价机器</p></body>
频道一,http://x/1
频道二,rtmp://x/2
频道三,udp://@239.1.1.1:1234
`)
	got := ParseTXT(raw)
	if len(got) != 3 {
		t.Fatalf("ParseTXT = %#v，期望只剩 3 个真实频道", got)
	}
	for i, want := range []string{"频道一", "频道二", "频道三"} {
		if got[i].Name != want {
			t.Fatalf("第 %d 条 = %q，期望 %q", i, got[i].Name, want)
		}
	}
}
