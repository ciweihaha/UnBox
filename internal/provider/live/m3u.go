// Package live 实现 M1 的 IPTV 直播来源：M3U/TXT 解析 + Provider 适配。
package live

import (
	"bytes"
	"strings"
)

// Entry 是 M3U/TXT 播放列表中的一条媒体条目。
type Entry struct {
	Name  string
	URL   string
	Logo  string
	Group string
	ID    string // tvg-id
}

// splitLines 按行切分播放列表：不再用 bufio.Scanner——它有行长上限，
// 超长行会让 Scan 直接停掉、静默丢掉该行之后的所有频道。输入本身已
// 整体读进内存（上游按 64MB 封顶），按换行切分不新增内存风险。
func splitLines(raw []byte) []string {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	parts := bytes.Split(raw, []byte("\n"))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(string(p)))
	}
	return out
}

// ParseM3U 解析 #EXTM3U 播放列表。#EXTINF 行中的 tvg-id / tvg-logo /
// group-title 属性被提取，逗号后的标题作为 Name。容错：剥 BOM、容忍
// CRLF、跳过空行与 # 注释行、无 #EXTINF 前导的 URL 跳过、#EXTINF 后
// 没有 URL 的条目丢弃。
func ParseM3U(raw []byte) []Entry {
	var out []Entry
	var cur *Entry // 当前待配对 URL 的 #EXTINF
	for _, line := range splitLines(raw) {
		if line == "" || strings.HasPrefix(line, "#EXTM3U") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#EXTINF") {
				e := parseExtinf(line)
				cur = &e
			}
			continue
		}
		// URL 行：必须已有 #EXTINF 前导才配对
		if cur != nil {
			cur.URL = line
			out = append(out, *cur)
			cur = nil
		}
	}
	return out
}

// parseExtinf 解析形如 `#EXTINF:-1 attr="v" attr2="v2",Name` 的行。
//
// 标题分隔逗号取第一个不在引号内的逗号：属性值里可能含逗号
// （如 group-title="综艺,电影"），按第一个逗号切会把标题切碎。
// 属性用词法扫描提取，避免子串命中：x-tvg-id 不是 tvg-id，logo URL
// 查询串里的 group-title= 也不是 group-title 属性。
func parseExtinf(line string) Entry {
	var e Entry
	rest := strings.TrimPrefix(line, "#EXTINF")
	if i := indexTitleComma(rest); i >= 0 {
		e.Name = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	attrs := parseExtinfAttrs(rest)
	e.ID = attrs["tvg-id"]
	e.Logo = attrs["tvg-logo"]
	e.Group = attrs["group-title"]
	return e
}

// indexTitleComma 返回第一个不在引号内的逗号下标；没有返回 -1。
func indexTitleComma(s string) int {
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case ',':
			return i
		}
	}
	return -1
}

// parseExtinfAttrs 把 `#EXTINF` 的属性段扫成 key→value：
// `key="v"`（值内可含空格、逗号、=）与 `key=v` 两种形态；时长段
// （`:-1`，无 =）跳过。只在词法边界处认 key，避免子串误命中。
func parseExtinfAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		j := i
		for j < len(s) && s[j] != '=' && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		key := s[i:j]
		if j >= len(s) || s[j] != '=' {
			i = j // 不是 key=value（如时长段），跳过该词
			continue
		}
		j++ // 跳过 =
		var value string
		if j < len(s) && (s[j] == '"' || s[j] == '\'') {
			quote := s[j]
			if k := strings.IndexByte(s[j+1:], quote); k >= 0 {
				value = s[j+1 : j+1+k]
				i = j + 1 + k + 1
			} else {
				value = s[j+1:] // 引号未闭合，取剩余部分
				i = len(s)
			}
		} else {
			k := j
			for k < len(s) && s[k] != ' ' && s[k] != '\t' {
				k++
			}
			value = s[j:k]
			i = k
		}
		if key != "" {
			attrs[key] = value
		}
	}
	return attrs
}

// ParseTXT 解析「名称,URL」每行一条的简单 TXT 播放列表。
func ParseTXT(raw []byte) []Entry {
	var out []Entry
	for _, line := range splitLines(raw) {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, url, ok := strings.Cut(line, ",")
		name = strings.TrimSpace(name)
		url = strings.TrimSpace(url)
		if !ok || name == "" || url == "" {
			continue
		}
		// 只接受带协议头的地址。HTML 网页里含逗号的行（meta/script/文本）会被
		// 切成「名字,伪地址」，不校验就会把网页当成一堆垃圾频道导进来。
		if !hasURLScheme(url) {
			continue
		}
		out = append(out, Entry{Name: name, URL: url})
	}
	return out
}

// hasURLScheme 判断地址是否带协议头（http://、rtmp://、udp:// 等）。
func hasURLScheme(raw string) bool {
	idx := strings.Index(raw, "://")
	if idx <= 0 {
		return false
	}
	for i, r := range raw[:idx] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.') && i > 0:
		default:
			return false
		}
	}
	return true
}
