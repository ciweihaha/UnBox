package rule

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 真实规则（斗鱼直播）的 POST请求数据 是对象 {"sk":"{wd}","offset":0,...}，
// 不是字符串。XYQ.PostBody 是 FlexString，直接 Unmarshal 不会把整站规则判废，
// 对象形态会被序列化成紧凑 JSON 字符串。
func TestParseXYQPostBodyObject(t *testing.T) {
	const raw = `{
  "规则名": "斗鱼直播",
  "首页推荐链接": "https://www.douyu.com",
  "搜索链接": "https://api.example/search",
  "POST请求数据": {"sk": "{wd}", "offset": 0, "limit": 20}
}`
	r, err := ParseXYQ([]byte(raw))
	if err != nil {
		t.Fatalf("对象形态 POST请求数据 应能解析: %v", err)
	}
	got := string(r.PostBody)
	if got == "" {
		t.Fatal("PostBody 为空")
	}
	// 转成 JSON 字符串后应保留 {wd} 占位符，供 Search 替换。
	if !containsStr(got, `{wd}`) {
		t.Fatalf("PostBody = %q，应含 {wd} 占位符", got)
	}
	// 必须是合法 JSON（对象被紧凑序列化了）。
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("PostBody = %q 不是合法 JSON: %v", got, err)
	}
	sk, _ := obj["sk"].(string)
	if sk != "{wd}" {
		t.Fatalf("obj.sk = %q，期望 {wd}", sk)
	}
	// {wd} 替换仍应工作：Search 构造 POST 体时把占位符换成关键词。
	replaced := strings.ReplaceAll(got, "{wd}", "火影")
	if !containsStr(replaced, `"火影"`) {
		t.Fatalf("替换失败: %q", replaced)
	}
}

// 字符串形态（影搜等）不能回归。
func TestParseXYQPostBodyString(t *testing.T) {
	const raw = `{"首页推荐链接":"https://x.com","搜索链接":"https://x/s;post","POST请求数据":"{\"key\":\"{wd}\"}"}`
	r, err := ParseXYQ([]byte(raw))
	if err != nil {
		t.Fatalf("字符串 POST请求数据 应能解析: %v", err)
	}
	if string(r.PostBody) != `{"key":"{wd}"}` {
		t.Fatalf("PostBody = %q，期望原样保留", string(r.PostBody))
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// 搜索关键词含 +、&、= 时，表单 POST 体里的 {wd} 必须按
// x-www-form-urlencoded 转义，否则 &、= 会被服务端当成分隔符，+ 会被读成空格。
func TestXYQSearchPostBodyEscapesFormQuery(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"code":200,"data":[]}`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:        srv.URL,
		SearchURL:      srv.URL + "/search;post",
		PostBody:       `wd={wd}&page=1`,
		SearchArray:    "data",
		SearchTitle:    "title",
		SearchLink:     "key",
		SearchModeJSON: "0",
	}, NewClient(nil))

	if _, err := e.Search(context.Background(), "C++&a=b"); err != nil {
		t.Fatal(err)
	}
	if gotBody != "wd=C%2B%2B%26a%3Db&page=1" {
		t.Fatalf("POST 体 = %q，期望转义后的查询", gotBody)
	}
}

// 搜索关键词含引号/反斜杠时，JSON POST 体里的 {wd} 必须按 JSON 字符串
// 转义，否则拼出的请求体不是合法 JSON，服务端直接拒收。
func TestXYQSearchPostBodyEscapesJSONQuery(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"code":200,"data":[]}`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:        srv.URL,
		SearchURL:      srv.URL + "/search;post",
		PostBody:       `{"sk":"{wd}","offset":0}`,
		SearchArray:    "data",
		SearchTitle:    "title",
		SearchLink:     "key",
		SearchModeJSON: "0",
	}, NewClient(nil))

	if _, err := e.Search(context.Background(), `a"b\c`); err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(gotBody), &obj); err != nil {
		t.Fatalf("POST 体不是合法 JSON: %q (%v)", gotBody, err)
	}
	if obj["sk"] != `a"b\c` {
		t.Fatalf("sk = %v，期望原样关键词", obj["sk"])
	}
}
