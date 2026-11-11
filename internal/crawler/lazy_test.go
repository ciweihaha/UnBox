package crawler

import "testing"

func TestResolveLazy(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:'js:input={url:input.split("?")[0]}'}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.resolveLazy("线路", "a.m3u8?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a.m3u8" {
		t.Fatalf("resolveLazy=%q", got)
	}
}

func TestResolveLazyStringResult(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:'js:input=input.split("?")[0]'}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.resolveLazy("线路", "a.m3u8?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a.m3u8" {
		t.Fatalf("resolveLazy=%q", got)
	}
}

// TestResolveLazyObjectWithoutURL 是回归测试（第 30 轮）：lazy 脚本返回
// 不含 url 的对象时，不得把 "[object Object]" 当播放地址返回——
// 下游只会报误导性的地址无效，应直接报 lazy 未返回播放地址。
func TestResolveLazyObjectWithoutURL(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:'js:input={parse:1}'}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.resolveLazy("线路", "a.m3u8?x=1")
	if err == nil {
		t.Fatalf("无 url 的对象应报错，实际返回 %q", got)
	}
	if got == "[object Object]" {
		t.Fatalf("不应把 [object Object] 当播放地址返回")
	}
}

// TestResolveLazyStringObject 是边界测试（第 30 轮）：String 对象
// （new String(url)）仍应按字符串取回，不被上一条的修复误杀。
func TestResolveLazyStringObject(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:'js:input=new String("http://x/a.m3u8")'}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.resolveLazy("线路", "a.m3u8?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://x/a.m3u8" {
		t.Fatalf("resolveLazy=%q", got)
	}
}

// 规则脚本常用 `let html=...` / `const x=...` 开头。同一个引擎第二次执行同一条
// 脚本时，直接跑在共享 VM 的全局作用域里会命中
// "Identifier 'html' has already been declared"（360影视详情第二次就挂）。
func TestResolveLazyRepeatedRunsDoNotCollide(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:"js:let base=input.url.split('?')[0]; input=base"}`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := e.resolveLazy("", "http://x/a.m3u8?tok=1")
		if err != nil {
			t.Fatalf("第 %d 次 lazy 执行失败: %v", i+1, err)
		}
		if got != "http://x/a.m3u8" {
			t.Fatalf("第 %d 次 got %q", i+1, got)
		}
	}
}

func TestExtractJSDetailRepeatedRunsDoNotCollide(t *testing.T) {
	e := New()
	script := `let html = {title:"甲"}; const tag = "x"; VOD.vod_name = html.title + tag`
	rule := &Rule{Host: "https://example.com"}
	for i := 0; i < 3; i++ {
		detail, err := e.extractJSDetail("", script, rule, "1")
		if err != nil {
			t.Fatalf("第 %d 次详情 JS 执行失败: %v", i+1, err)
		}
		if detail.VodName != "甲x" {
			t.Fatalf("第 %d 次 VodName=%q", i+1, detail.VodName)
		}
	}
}
