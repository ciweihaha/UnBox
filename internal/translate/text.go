// Package translate 的字幕文本小工具：免费接口返回的译文清洗
// 与「已是中文就不翻译」的判定，各免费接口客户端共用。
package translate

import (
	"strings"
	"unicode"
)

// cleanTranslation 去掉接口常带的多余包装：首尾空白、
// 整句引号（英文 " 与中文 “”）、「翻译：/Translation:」前缀。
func cleanTranslation(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"翻译：", "翻译:", "Translation:", "translation:"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
			break
		}
	}
	if len(s) >= 2 {
		runes := []rune(s)
		first, last := runes[0], runes[len(runes)-1]
		if (first == '"' && last == '"') || (first == '“' && last == '”') {
			s = strings.TrimSpace(string(runes[1 : len(runes)-1]))
		}
	}
	return s
}

// isChineseText 判一段文本是否含中文：含中文就不翻译
// （已是中文或双语字幕直接显示原文）。
func isChineseText(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
