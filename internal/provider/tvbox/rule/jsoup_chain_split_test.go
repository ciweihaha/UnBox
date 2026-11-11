package rule

import (
	"reflect"
	"testing"
)

// TestJsoupChainKeepsDoubleAmpersandInHas 是回归测试（第 26 轮）：
// 链式规则按 && 切分时，:has() 里的引号属性值含 && 也不能把链切散。
// （:matches() 正则里的 && 是另一处问题——外层 HTML 会把 & 转义成
// &amp;，见下一轮；这里用 :has 隔离，只验证切分本身。）
func TestJsoupChainKeepsDoubleAmpersandInHas(t *testing.T) {
	doc := jsoupDoc(t, `<div><a href="/p?a=1&&b=2">T</a></div><div><a href="/other">O</a></div>`)
	got := jsoupExtractFirst(doc, `div:has(a[href='/p?a=1&&b=2'])&&Text`)
	want := []string{"T"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("jsoupExtractFirst = %#v, want %#v", got, want)
	}
}

// TestJsoupChainKeepsDoubleAmpersandInQuotedAttr 是回归测试（第 26 轮）：
// 属性选择器引号里的 &&（如 href 查询串）同样不能切散链。
func TestJsoupChainKeepsDoubleAmpersandInQuotedAttr(t *testing.T) {
	doc := jsoupDoc(t, `<a href="/p?a=1&&b=2">Link</a><a href="/other">Other</a>`)
	got := jsoupExtractFirst(doc, `a[href='/p?a=1&&b=2']&&Text`)
	want := []string{"Link"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("jsoupExtractFirst = %#v, want %#v", got, want)
	}
}

// TestJsoupSelectNodesKeepsDoubleAmpersandInHas 同上，覆盖只取节点
// 的数组路径（jsoupSelectOn）。
func TestJsoupSelectNodesKeepsDoubleAmpersandInHas(t *testing.T) {
	doc := jsoupDoc(t, `<div class="item"><a href="/p?a=1&&b=2">T</a></div><div class="item">plain</div>`)
	nodes := jsoupSelectNodes(doc, `div.item:has(a[href='/p?a=1&&b=2'])`)
	if nodes == nil || nodes.Length() != 1 || nodes.Text() != "T" {
		t.Fatalf("jsoupSelectNodes 命中 %v，want 1 个 T 节点", nodes)
	}
}
