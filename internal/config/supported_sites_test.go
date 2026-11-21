package config

import "testing"

// TestCountSupportedSites：支持格式才计数；不支持的格式不计数。
// 口径与 internal/shell.collectVodSites 一致。
func TestCountSupportedSites(t *testing.T) {
	// agg 风格的列表响应：不是 TVBox 配置，0 站点。
	aggLike, err := Parse([]byte(`{"code":1,"msg":"ok","class":[{"type_id":1,"type_name":"电影"}],"list":[{"vod_id":1,"vod_name":"寻梦环游记"}]}`))
	if err != nil {
		t.Fatalf("Parse(agg 列表响应): %v", err)
	}
	if got := CountSupportedSites([]*Config{aggLike}); got != 0 {
		t.Fatalf("agg 列表响应支持站点数 = %d，想要 0", got)
	}

	// 混合配置：1 个 CMS + 1 个 drpy(http) + 1 个 js + 1 个 xpath + 1 个 python。
	mixed, err := Parse([]byte(`{"sites":[
		{"key":"a","name":"CMS站","type":1,"api":"https://example.com/api"},
		{"key":"b","name":"drpy站","type":3,"api":"https://example.com/rule.js"},
		{"key":"c","name":"js站","type":3,"api":"https://example.com/spider.js"},
		{"key":"d","name":"xpath站","type":0,"api":"https://example.com/xpath"},
		{"key":"e","name":"py站","type":3,"api":"https://example.com/spider.py"}
	]}`))
	if err != nil {
		t.Fatalf("Parse(混合配置): %v", err)
	}
	// drpy 的 api 是 .js 后缀会判成 js；xpath/python 不支持。
	// 想要：CMS 1 + js 2 = 3（b、c 都是 js），xpath/python 0。
	if got := CountSupportedSites([]*Config{mixed}); got != 3 {
		t.Fatalf("混合配置支持站点数 = %d，想要 3", got)
	}

	// CMS 站点没 api 地址不算支持。
	noAPI, err := Parse([]byte(`{"sites":[{"key":"a","name":"没地址","type":1,"api":""}]}`))
	if err != nil {
		t.Fatalf("Parse(没 api 的 CMS): %v", err)
	}
	if got := CountSupportedSites([]*Config{noAPI}); got != 0 {
		t.Fatalf("没 api 的 CMS 支持站点数 = %d，想要 0", got)
	}

	// 空配置与 nil 容错。
	if got := CountSupportedSites(nil); got != 0 {
		t.Fatalf("nil 支持站点数 = %d，想要 0", got)
	}
	if got := CountSupportedSites([]*Config{nil}); got != 0 {
		t.Fatalf("[nil] 支持站点数 = %d，想要 0", got)
	}
}
