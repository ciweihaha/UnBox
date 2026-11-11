package rule

import (
	"reflect"
	"testing"
)

// TestJsoupMatchesUnescapesEntities 是回归测试（第 27 轮）：
// :matches(正则) 按元素外层 HTML 匹配，但 goquery 序列化会把 &
// 转义成 &amp;——含 & 的正则（如文本里的 a&&b、链接查询串 a=1&b=2）
// 永远命中不了。匹配前应对 HTML 反转义，还原规则作者写的字面形态。
func TestJsoupMatchesUnescapesEntities(t *testing.T) {
	doc := jsoupDoc(t, `<div>a&&b</div><div>plain</div>`)
	got := jsoupExtractFirst(doc, `div:matches(a&&b)&&Text`)
	want := []string{"a&&b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(":matches(a&&b) = %#v, want %#v", got, want)
	}
}

// TestJsoupMatchesQueryString 同上：链接查询串里的 & 也应按字面匹配。
func TestJsoupMatchesQueryString(t *testing.T) {
	doc := jsoupDoc(t, `<a href="/p?a=1&b=2">Link</a><a href="/other">Other</a>`)
	got := jsoupExtractFirst(doc, `a:matches(a=1&b=2)&&Text`)
	want := []string{"Link"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(":matches(a=1&b=2) = %#v, want %#v", got, want)
	}
}

// TestJsoupNotMatchesUnescapesEntities 同上，覆盖 :not(:matches(...))
// 的取反路径：含 & 的项应被正确排除。
func TestJsoupNotMatchesUnescapesEntities(t *testing.T) {
	doc := jsoupDoc(t, `<div class="item">a&&b</div><div class="item">plain</div>`)
	got := jsoupExtractFirst(doc, `div.item:not(:matches(a&&b))&&Text`)
	want := []string{"plain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(":not(:matches(a&&b)) = %#v, want %#v", got, want)
	}
}
