package crawler

import "testing"

func TestParseClasses(t *testing.T) {
	html := `<ul><li><a href="/list/movie.html">电影</a></li><li><a href="/list/tv.html">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li&&a&&Text;li&&a&&Text;li&&a&&href;/(\w+)\.html`)
	if err != nil || len(cls) != 2 || cls[0].TypeID != "movie" || cls[0].TypeName != "电影" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

func TestParseClassesUsesSelectedEntries(t *testing.T) {
	html := `<div><a href="/other.html">无关</a></div><ul><li><a href="/list/movie.html">电影</a></li><li><a href="/list/tv.html">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li&&a&&Text;li&&a&&Text;li&&a&&href;/(\w+)\.html`)
	if err != nil || len(cls) != 2 || cls[0].TypeName != "电影" || cls[0].TypeID != "movie" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

func TestParseClassesRequiresSegments(t *testing.T) {
	// 2 段太少了。
	if _, err := parseClasses(`<li>x</li>`, `li;Text`); err == nil {
		t.Fatal("expected error for missing class_parse segment")
	}
	// 第 4 段是 id 正则：分号后的内容归入正则，非法正则是配置错误。
	if _, err := parseClasses(`<li>x</li>`, `li;Text;href;/(x)/;[invalid`); err == nil {
		t.Fatal("expected error for invalid class_parse id regexp")
	}
}

// TestParseClassesKeepsSemicolonInIDRegexp 是回归测试（第 33 轮）：
// 第 4 段 id 正则里的分号（如 id=12;v=7 的提取）不能把规则多切出
// 一段而报错，4 段后的分号都属于正则本身。
func TestParseClassesKeepsSemicolonInIDRegexp(t *testing.T) {
	html := `<ul><li><a href="/c?id=12;v=7">电影</a></li><li><a href="/c?id=34;v=8">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li&&a&&Text;li&&a&&Text;li&&a&&href;id=(\d+);v=(\d+)`)
	if err != nil || len(cls) != 2 || cls[0].TypeID != "12" || cls[0].TypeName != "电影" || cls[1].TypeID != "34" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

// 三段省略 id 正则：很多 drpy2 真实脚本（如 B站影视）的 id 规则已直接
// 取出值，不必再配 /…/ 提取正则。
func TestParseClassesThreeSegmentsWithoutRegexp(t *testing.T) {
	html := `<ul><li><a href="/movie.html" data-id="17">电影</a></li><li><a href="/tv.html" data-id="18">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li;a&&Text;a&&data-id`)
	if err != nil || len(cls) != 2 || cls[0].TypeID != "17" || cls[0].TypeName != "电影" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

func TestParseClassesSupportsJQueryLimit(t *testing.T) {
	html := `<ul class="nav"><li><a href="/movie.html">电影</a></li><li><a href="/tv.html">电视剧</a></li><li><a href="/anime.html">动漫</a></li></ul>`
	cls, err := parseClasses(html, `.nav&&li:lt(2);a&&Text;a&&href;/([a-z]+)\.html`)
	if err != nil || len(cls) != 2 || cls[1].TypeID != "tv" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

// TestParseClassesKeepsDoubleAmpersandInQuotedAttr 是回归测试（第 28 轮）：
// 选择器引号里的 &&（如 href 查询串）不能把选择链切散，否则
// selectClassEntries 拼出非法选择器、条目取不到名字与 id。
// 与 evalRule、JSoup 链按引号切分是同一类问题。
func TestParseClassesKeepsDoubleAmpersandInQuotedAttr(t *testing.T) {
	html := `<div class="nav"><a href="/c?a=1&&b=2">电影</a><a href="/c?a=3&&b=4">电视剧</a></div>`
	cls, err := parseClasses(html, `.nav&&a[href*='a=1&&b=2'];a&&Text;a&&href`)
	if err != nil || len(cls) != 1 || cls[0].TypeID != "/c?a=1&&b=2" || cls[0].TypeName != "电影" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}
