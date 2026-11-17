package live

import (
	"context"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// TestRepairMojibakeCP437：UTF-8 字节被当成 CP437 解码后又存成 UTF-8 的
// 双重编码乱码要能修回中文；干净文本、纯 ASCII 不动。
func TestRepairMojibakeCP437(t *testing.T) {
	cases := []struct{ in, want string }{
		// 云电脑上真实存坏的数据：综合 的 UTF-8 字节被按 CP437 读成 τ╗╝σÉê。
		{"CCTVτ╗╝σÉê", "CCTV综合"},
		{"CCTVτ╗╝", "CCTV综"},
		{"σÉêσÉê", "合合"},
		// 干净文本不动。
		{"CCTV综合", "CCTV综合"},
		{"广东卫视", "广东卫视"},
		{"CCTV-1", "CCTV-1"},
		{"", ""},
		// 合法但含 CP437 字符的文本不动（单字节无法拼成合法 UTF-8，不触发修复）。
		{"café", "café"},
	}
	for _, c := range cases {
		if got := repairMojibake(c.in); got != c.want {
			t.Errorf("repairMojibake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParseM3UGBK：GBK 编码的 M3U（不少中文源是 GBK）要能正确解析出中文名。
func TestParseM3UGBK(t *testing.T) {
	gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("#EXTM3U\n#EXTINF:-1 group-title=\"央视频道\",CCTV综合\nhttp://example.com/cctv.m3u8\n"))
	if err != nil {
		t.Fatal(err)
	}
	entries := ParseM3U(gbk)
	if len(entries) != 1 {
		t.Fatalf("ParseM3U GBK entries = %d, want 1", len(entries))
	}
	if entries[0].Name != "CCTV综合" || entries[0].Group != "央视频道" {
		t.Errorf("ParseM3U GBK entry = %+v, want Name=CCTV综合 Group=央视频道", entries[0])
	}
}

// TestParseM3URepairsMojibake：M3U 里已经是 CP437 乱码的频道名/分组名，解析时修回中文。
func TestParseM3URepairsMojibake(t *testing.T) {
	raw := []byte("#EXTM3U\n#EXTINF:-1 group-title=\"τ╗╝σÉê\",CCTVτ╗╝σÉê\nhttp://example.com/cctv.m3u8\n")
	entries := ParseM3U(raw)
	if len(entries) != 1 {
		t.Fatalf("ParseM3U mojibake entries = %d, want 1", len(entries))
	}
	if entries[0].Name != "CCTV综合" || entries[0].Group != "综合" {
		t.Errorf("ParseM3U mojibake entry = %+v, want Name=CCTV综合 Group=综合", entries[0])
	}
}

// TestNewRepairsMojibake：已存坏的频道（库里就是乱码）进 Provider 时修回中文，
// 界面直接显示正常，不用用户重新导入。
func TestNewRepairsMojibake(t *testing.T) {
	p := New([]config.Channel{{Name: "CCTVτ╗╝σÉê", Group: "τ╗╝σÉê", URLs: []string{"http://example.com/cctv.m3u8"}}})
	sections, err := p.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || sections[0].Title != "综合" {
		t.Errorf("New mojibake groups = %+v, want [综合]", sections)
	}
	page, err := p.Browse(context.Background(), "综合", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "CCTV综合" {
		t.Errorf("New mojibake items = %+v, want [CCTV综合]", page.Items)
	}
}
