package crawler

import (
	"testing"
	"time"
)

func TestHelpersMd5Base64(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ return md5("abc") + "|" + base64Encode("hi") + "|" + base64Decode("aGk=") }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "900150983cd24fb0d6963f7d28e17f72|aGk=|hi" {
		t.Fatalf("got %q", got)
	}
}

func TestHelpersCookieAndHeader(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ cookie("sid", "abc"); header("X-Test", "yes"); return cookie("sid") + "|" + header("X-Test") }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil || v.String() != "abc|yes" {
		t.Fatalf("got %v, %v", v, err)
	}
}

// TestSetIntervalHugeDelayClampsToMax 是回归测试（第 43 轮）：
// setInterval 的延迟先乘毫秒再钳，超大值溢出变负后被当成 0 延迟，
// 回调在无等待的空转里连跑；应像 sleep/setTimeout 那样先钳毫秒数，
// 超大延迟按上限等待。
func TestSetIntervalHugeDelayClampsToMax(t *testing.T) {
	e := New()
	script := `var calls = 0; setInterval(function(id){ calls++; clearInterval(id); }, 10000000000000)`
	start := time.Now()
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if got := e.vm.Get("calls").ToInteger(); got != 1 {
		t.Fatalf("calls = %d，期望 1", got)
	}
	if elapsed < maxTimerDelay {
		t.Fatalf("超大延迟应按上限等待 %s，实际只等了 %s", maxTimerDelay, elapsed)
	}
}
