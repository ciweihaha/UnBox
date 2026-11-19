package shell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testVodConfigJSON 是一份最小 TVBox 点播配置：一个 CMS 站点。
const testVodConfigJSON = `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://example.com/api"}]}`

func newVodConfigServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testVodConfigJSON))
	}))
}

// newVodBodyServer 返回固定 body 的测试服务器（目录 JSON 用，不走 TVBox 配置）。
func newVodBodyServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestVodSourceStatusDefaults(t *testing.T) {
	svc := newTestService(t)
	st := svc.GetVodSourceStatus()
	if st.Kind != VodSourceLocal || st.LocalRef != "" || st.CloudflareURL != "" || st.DirectoryURL != "" || st.UpdatedAt != 0 || st.Sites != 0 {
		t.Fatalf("默认状态 = %+v，想要 local/空/0", st)
	}
}

func TestSetCloudflareVodURLRejectsNonHTTP(t *testing.T) {
	svc := newTestService(t)
	for _, bad := range []string{"", "not a url", "/tmp/vod.json", "file:///tmp/vod.json", "ftp://example.com/vod.json"} {
		if _, err := svc.SetCloudflareVodURL(bad); err == nil {
			t.Fatalf("SetCloudflareVodURL(%q) 成功，想要错误（只要 http(s) 地址）", bad)
		}
	}
}

func TestSetCloudflareVodURLImportsAndPersists(t *testing.T) {
	srv := newVodConfigServer(t)
	t.Cleanup(srv.Close)
	svc := newTestService(t)
	res, err := svc.SetCloudflareVodURL(srv.URL + "/vod.json")
	if err != nil {
		t.Fatalf("SetCloudflareVodURL: %v", err)
	}
	if res.Sites != 1 {
		t.Fatalf("导入站点数 = %d，想要 1", res.Sites)
	}
	st := svc.GetVodSourceStatus()
	if st.Kind != VodSourceCloudflare || st.CloudflareURL != srv.URL+"/vod.json" || st.Sites != 1 {
		t.Fatalf("状态 = %+v，想要 cloudflare/该地址/1 站", st)
	}
	if st.UpdatedAt == 0 || time.Since(time.Unix(st.UpdatedAt, 0)) > time.Minute {
		t.Fatalf("UpdatedAt = %d，想要最近的时间", st.UpdatedAt)
	}
	// 换一个服务实例读同一 store：来源与地址要能读回。
	svc2 := newPlaybackSettingsService(t, svc.store)
	st2 := svc2.GetVodSourceStatus()
	if st2.Kind != VodSourceCloudflare || st2.CloudflareURL != srv.URL+"/vod.json" {
		t.Fatalf("重读状态 = %+v，想要 cloudflare/该地址", st2)
	}
}

func TestRefreshCloudflareVodSource(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.RefreshCloudflareVodSource(); err == nil {
		t.Fatal("未填 CF 地址时 RefreshCloudflareVodSource 成功，想要错误")
	}
	srv := newVodConfigServer(t)
	t.Cleanup(srv.Close)
	if _, err := svc.SetCloudflareVodURL(srv.URL + "/vod.json"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.RefreshCloudflareVodSource()
	if err != nil {
		t.Fatalf("RefreshCloudflareVodSource: %v", err)
	}
	if res.Sites != 1 {
		t.Fatalf("刷新后站点数 = %d，想要 1", res.Sites)
	}
}

func TestUseLocalVodSource(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.UseLocalVodSource(); err == nil {
		t.Fatal("未导入过本地源时 UseLocalVodSource 成功，想要错误")
	}
	path := filepath.Join(t.TempDir(), "vod.json")
	if err := os.WriteFile(path, []byte(testVodConfigJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportVodSource(path); err != nil {
		t.Fatalf("ImportVodSource(本地文件): %v", err)
	}
	st := svc.GetVodSourceStatus()
	if st.Kind != VodSourceLocal || st.LocalRef != path || st.Sites != 1 {
		t.Fatalf("本地导入后状态 = %+v，想要 local/该文件/1 站", st)
	}
	srv := newVodConfigServer(t)
	t.Cleanup(srv.Close)
	if _, err := svc.SetCloudflareVodURL(srv.URL + "/vod.json"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetVodSourceStatus().Kind; got != VodSourceCloudflare {
		t.Fatalf("切 CF 后 Kind = %q，想要 cloudflare", got)
	}
	res, err := svc.UseLocalVodSource()
	if err != nil {
		t.Fatalf("UseLocalVodSource: %v", err)
	}
	if res.Sites != 1 {
		t.Fatalf("切回本地后站点数 = %d，想要 1", res.Sites)
	}
	st = svc.GetVodSourceStatus()
	if st.Kind != VodSourceLocal {
		t.Fatalf("切回本地后 Kind = %q，想要 local", st.Kind)
	}
}

// 删掉的正是已填的 CF 地址时，CF 入口一并清空、来源回到本地。
func TestDeleteSourceClearsCloudflareURL(t *testing.T) {
	srv := newVodConfigServer(t)
	t.Cleanup(srv.Close)
	svc := newTestService(t)
	url := srv.URL + "/vod.json"
	if _, err := svc.SetCloudflareVodURL(url); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSource("vod", url); err != nil {
		t.Fatal(err)
	}
	st := svc.GetVodSourceStatus()
	if st.CloudflareURL != "" || st.Kind != VodSourceLocal {
		t.Fatalf("删 CF 源后状态 = %+v，想要地址清空、回到 local", st)
	}
}

func TestCfVodDirectoryURLDefaultAndRoundTrip(t *testing.T) {
	svc := newTestService(t)

	if got := svc.GetCfVodDirectoryURL(); got != DefaultCfVodDirectoryURL {
		t.Fatalf("未设置时 = %q, want 默认目录地址 %q", got, DefaultCfVodDirectoryURL)
	}
	dir := "https://example.com/my-sources.json"
	if err := svc.SetCfVodDirectoryURL(dir); err != nil {
		t.Fatalf("SetCfVodDirectoryURL: %v", err)
	}
	if got := svc.GetCfVodDirectoryURL(); got != dir {
		t.Fatalf("设置后 = %q, want %q", got, dir)
	}
}

func TestSetCfVodDirectoryURLRejectsNonHTTP(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetCfVodDirectoryURL("ftp://example.com/x.json"); err == nil {
		t.Fatal("非 http(s) 目录地址应该被拒绝")
	}
	if err := svc.SetCfVodDirectoryURL(""); err == nil {
		t.Fatal("空目录地址应该被拒绝")
	}
}

func TestFetchCfVodDirectory(t *testing.T) {
	dir := newVodBodyServer(t, `{"sources":[
		{"name":"聚合万能源","url":"https://live.ciweihaha.win/vod.json"},
		{"name":"聚合·37站","url":"https://ysp.ciweihaha.win/agg"},
		{"name":"","url":"https://bad.example/x.json"},
		{"name":"坏地址","url":"ftp://bad.example/x.json"}
	]}`)
	svc := newTestService(t)
	if err := svc.SetCfVodDirectoryURL(dir.URL + "/sources.json"); err != nil {
		t.Fatalf("SetCfVodDirectoryURL: %v", err)
	}
	got, err := svc.FetchCfVodDirectory()
	if err != nil {
		t.Fatalf("FetchCfVodDirectory: %v", err)
	}
	if len(got) != 2 || got[0].Name != "聚合万能源" || got[1].URL != "https://ysp.ciweihaha.win/agg" {
		t.Fatalf("拉取结果 = %#v, want 前两项有效源", got)
	}
}

func TestFetchCfVodDirectoryAcceptsBareArray(t *testing.T) {
	dir := newVodBodyServer(t, `[{"name":"甲","url":"https://a.example/vod.json"}]`)
	svc := newTestService(t)
	if err := svc.SetCfVodDirectoryURL(dir.URL + "/sources.json"); err != nil {
		t.Fatalf("SetCfVodDirectoryURL: %v", err)
	}
	got, err := svc.FetchCfVodDirectory()
	if err != nil {
		t.Fatalf("FetchCfVodDirectory: %v", err)
	}
	if len(got) != 1 || got[0].Name != "甲" {
		t.Fatalf("裸数组结果 = %#v, want 1 项", got)
	}
}

func TestFetchCfVodDirectoryErrors(t *testing.T) {
	svc := newTestService(t)
	bad := newVodBodyServer(t, `not json`)
	if err := svc.SetCfVodDirectoryURL(bad.URL + "/sources.json"); err != nil {
		t.Fatalf("SetCfVodDirectoryURL: %v", err)
	}
	if _, err := svc.FetchCfVodDirectory(); err == nil {
		t.Fatal("非法 JSON 应该返回错误，不能静默当成空目录")
	}
}
