package crawler

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func evalRuleFirst(t *testing.T, html, rule string) string {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	values := evalRule(doc, rule)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// TestEvalRuleKeepsDoubleAmpersandInArgs 是回归测试（第 25 轮）：
// 规则链按 && 切分时，match()/replace() 参数里的 && 不能把链切散，
// 否则参数被截断、匹配结果出错（与 xpath 按引号切分是同一类问题）。
func TestEvalRuleKeepsDoubleAmpersandInArgs(t *testing.T) {
	got := evalRuleFirst(t, `<div>a&&b</div>`, `div&&Text()&&match(a&&b)`)
	if got != "a&&b" {
		t.Fatalf("match(a&&b) = %q, want %q", got, "a&&b")
	}
}

// TestEvalRuleKeepsCommaInQuotedArgs 是回归测试（第 25 轮）：
// replace() 的参数按逗号切分时，引号内的逗号不能切分参数，
// 否则替换不生效、标题里的逗号去不掉。
func TestEvalRuleKeepsCommaInQuotedArgs(t *testing.T) {
	got := evalRuleFirst(t, `<div>x a,b y</div>`, `div&&Text()&&replace('a,b','-')`)
	if got != "x - y" {
		t.Fatalf("replace('a,b','-') = %q, want %q", got, "x - y")
	}
}

// TestEvalRuleKeepsCommaInDoubleQuotedArgs 同上，双引号参数形态。
func TestEvalRuleKeepsCommaInDoubleQuotedArgs(t *testing.T) {
	got := evalRuleFirst(t, `<div>x a,b y</div>`, `div&&Text()&&replace("a,b","-")`)
	if got != "x - y" {
		t.Fatalf("replace(\"a,b\",\"-\") = %q, want %q", got, "x - y")
	}
}
