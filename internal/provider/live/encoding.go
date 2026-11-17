// Package live 的编码容错：直播源常是中文站提供的 M3U/TXT，编码五花八门。
//
// 两类乱码在这里统一处理：
//  1. GBK 源：整份字节不是合法 UTF-8，按 GBK 解码成 UTF-8。
//  2. CP437 二重编码：UTF-8 字节曾被按 IBM CP437（Windows 控制台默认代码页）
//     解码、又存回 UTF-8，中文变成希腊字母/制表符（如 综合 → τ╗╝σÉê）。
//     修法是把每个字符映回 CP437 字节，再按 UTF-8 解一次。
package live

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// decodePlaylistBytes 把播放列表原始字节解成 UTF-8：先剥 BOM；是合法 UTF-8
// 直接用；不是则按 GBK 解码（不少中文源是 GBK）；GBK 也解不动就原样返回，
// 交给上层按 UTF-8 容错（无效字节变 U+FFFD，不至于整份丢掉）。
func decodePlaylistBytes(raw []byte) []byte {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if utf8.Valid(raw) {
		return raw
	}
	if decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), raw); err == nil {
		return decoded
	}
	return raw
}

// repairMojibake 修 CP437 二重编码乱码：中文的 UTF-8 字节被按 CP437 读成
// 希腊字母/制表符等字符、又存成 UTF-8（如 综合 → τ╗╝σÉê）。把每个字符映回
// CP437 字节再按 UTF-8 解一次，就能还原。满足以下全部条件才修，避免误伤
// 正常文本：全部字符都能映到 CP437、映回的字节是合法 UTF-8、修完变了、
// 修完含中文（乱码源头就是中文）。
func repairMojibake(s string) string {
	if s == "" {
		return s
	}
	raw, _, err := transform.Bytes(charmap.CodePage437.NewEncoder(), []byte(s))
	if err != nil {
		return s // 含 CP437 没有的字符（如中文原文），不是这类乱码
	}
	if !utf8.Valid(raw) {
		return s
	}
	fixed := string(raw)
	if fixed == s || !hasCJK(fixed) {
		return s
	}
	return fixed
}

// hasCJK 判断字符串是否含中文字符。
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
