package translate

import "testing"

func TestCleanTranslation(t *testing.T) {
	cases := []struct{ in, want string }{
		{`  "你好"  `, "你好"},
		{"翻译：你好", "你好"},
		{"Translation: Hello", "Hello"},
		{"你好", "你好"},
		{"", ""},
	}
	for _, c := range cases {
		if got := cleanTranslation(c.in); got != c.want {
			t.Errorf("cleanTranslation(%q) = %q，想要 %q", c.in, got, c.want)
		}
	}
}

func TestIsChineseText(t *testing.T) {
	if !isChineseText("你好，世界") {
		t.Error("中文应判定为中文")
	}
	if !isChineseText("Hello 你好") {
		t.Error("中英混合应判定为中文")
	}
	if isChineseText("Hello, world") {
		t.Error("纯英文不应判定为中文")
	}
	if isChineseText("123...!?") {
		t.Error("纯符号不应判定为中文")
	}
	if isChineseText("") {
		t.Error("空文本不应判定为中文")
	}
}
