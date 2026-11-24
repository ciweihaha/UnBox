package shell

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/httpx"
	"github.com/unbox/unbox/internal/playback"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/provider/tvbox"
	"github.com/unbox/unbox/internal/store"
)

func newTestService(t *testing.T) *ShellService {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	channels := []config.Channel{
		{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}},
	}
	svc := NewShellService(live.New(channels), nil, s)
	t.Cleanup(func() { s.Close() })
	return svc
}

func newPlaybackSettingsService(t *testing.T, st *store.Store) *ShellService {
	t.Helper()
	svc := NewShellService(live.New(nil), nil, st)
	t.Cleanup(func() { _ = svc.ServiceShutdown() })
	return svc
}

func TestPlaybackSettingsRoundTripAndDefaults(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/playback-settings.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := newPlaybackSettingsService(t, st)
	if got := svc.GetPlaybackSettings(); got != (PlaybackSettings{}) {
		t.Fatalf("missing keys = %#v, want all false", got)
	}
	want := PlaybackSettings{AutoNext: true, AutoSwitchSource: true, PreloadNext: false}
	if err := svc.SetPlaybackSettings(want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetPlaybackSettings(); got != want {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestPlaybackSettingsInvalidValuesFallBackToFalse(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/playback-settings.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SetKV(playbackAutoNextKey, "not-a-bool"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKV(playbackAutoSwitchSourceKey, "true"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKV(playbackPreloadNextKey, "false"); err != nil {
		t.Fatal(err)
	}

	got := newPlaybackSettingsService(t, st).GetPlaybackSettings()
	want := PlaybackSettings{AutoSwitchSource: true}
	if got != want {
		t.Fatalf("invalid values = %#v, want %#v", got, want)
	}
}

func TestPlaybackSettingsStoreErrors(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/playback-settings.db")
	if err != nil {
		t.Fatal(err)
	}
	svc := newPlaybackSettingsService(t, st)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	if got := svc.GetPlaybackSettings(); got != (PlaybackSettings{}) {
		t.Fatalf("closed store read = %#v, want all false", got)
	}
	if err := svc.SetPlaybackSettings(PlaybackSettings{AutoNext: true}); err == nil {
		t.Fatal("closed store write should return an error")
	}
}
func TestThemeRoundTrip(t *testing.T) {
	svc := newTestService(t)
	theme, err := svc.GetTheme()
	if err != nil || theme != "" {
		t.Fatalf("默认主题应为空串, got %q, %v", theme, err)
	}
	if err := svc.SetTheme("aurora"); err != nil {
		t.Fatal(err)
	}
	theme, err = svc.GetTheme()
	if err != nil || theme != "aurora" {
		t.Fatalf("主题回读失败, got %q, %v", theme, err)
	}
}

func TestThemeEmptyService(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	if err := svc.SetTheme("aurora"); err != nil {
		t.Fatal(err)
	}
	theme, err := svc.GetTheme()
	if err != nil || theme != "" {
		t.Fatalf("无 store 应安全返回空串, got %q, %v", theme, err)
	}
}

func TestContentCardStyleRoundTrip(t *testing.T) {
	svc := newTestService(t)
	style, err := svc.GetContentCardStyle()
	if err != nil || style != "grid" {
		t.Fatalf("默认内容卡片样式应为 grid（Netflix 改版起默认海报卡片）, got %q, %v", style, err)
	}
	if err := svc.SetContentCardStyle("list"); err != nil {
		t.Fatal(err)
	}
	style, err = svc.GetContentCardStyle()
	if err != nil || style != "list" {
		t.Fatalf("明确存过 list 后应回读 list, got %q, %v", style, err)
	}
	if err := svc.SetContentCardStyle("grid"); err != nil {
		t.Fatal(err)
	}
	style, err = svc.GetContentCardStyle()
	if err != nil || style != "grid" {
		t.Fatalf("内容卡片样式回读失败, got %q, %v", style, err)
	}
}

func TestContentCardStyleInvalidValueFallsBackToList(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetContentCardStyle("cards"); err != nil {
		t.Fatal(err)
	}
	style, err := svc.GetContentCardStyle()
	if err != nil || style != "list" {
		t.Fatalf("非法内容卡片样式应回退 list, got %q, %v", style, err)
	}
}

func TestImportSubscriptionPlaylist(t *testing.T) {
	svc := newTestService(t)
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:-1 group-title=\"测试\",频道A\nhttp://x/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r, err := svc.ImportSubscription(path)
	if err != nil || r.Channels != 1 {
		t.Fatalf("ImportSubscription = %+v, %v", r, err)
	}
	gs, _ := svc.Groups()
	if len(gs) != 1 || gs[0] != "测试" {
		t.Fatalf("Groups = %v", gs)
	}
}

func TestGroupsAndChannels(t *testing.T) {
	svc := newTestService(t)
	gs, err := svc.Groups()
	if err != nil || len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("Groups = %v, %v", gs, err)
	}
	chs, err := svc.Channels("央视", 0)
	if err != nil || len(chs) != 1 || chs[0].Name != "CCTV-1" {
		t.Fatalf("Channels = %+v, %v", chs, err)
	}
}

// TestDeleteSourceUnloadsAndPersists：删源不能只删历史记录——源必须立即从
// 内存卸掉（当场不可用），且订阅快照要同步落盘，否则重启后
// RestoreSubscription 会把删掉的源原样恢复回来。
func TestDeleteSourceUnloadsAndPersists(t *testing.T) {
	svc := newTestService(t)
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:-1 group-title=\"测试\",频道A\nhttp://x/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r, err := svc.ImportSubscription(path)
	if err != nil || r.Channels != 1 {
		t.Fatalf("ImportSubscription = %+v, %v", r, err)
	}
	if err := svc.DeleteSource("live", path); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	gs, _ := svc.Groups()
	if len(gs) != 0 {
		t.Fatalf("删除后 Groups 应为空，got %v", gs)
	}
	// 模拟重启：同一 store 上新服务恢复订阅，删掉的源不应回来。
	svc2 := NewShellService(nil, nil, svc.store)
	r2, err := svc2.RestoreSubscription()
	if err != nil {
		t.Fatalf("RestoreSubscription: %v", err)
	}
	if r2.Channels != 0 || r2.LiveSources != 0 {
		t.Fatalf("重启恢复后删掉的源不应回来: %+v", r2)
	}
	gs2, _ := svc2.Groups()
	if len(gs2) != 0 {
		t.Fatalf("重启恢复后 Groups 应为空，got %v", gs2)
	}
}

func TestFavoriteRoundtrip(t *testing.T) {
	svc := newTestService(t)
	id := "央视/CCTV-1"
	if err := svc.AddFavorite(id); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	favs, err := svc.ListFavorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "CCTV-1" {
		t.Fatalf("ListFavorites = %+v, %v", favs, err)
	}
}

func TestPlayChannelRequiresPlayer(t *testing.T) {
	svc := newTestService(t)
	if err := svc.PlayChannel("央视/CCTV-1"); err == nil {
		t.Fatal("player 为 nil 时 PlayChannel 应报错")
	}
}

func TestGroupsBeforeImport(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	svc := NewShellService(nil, nil, s)
	gs, err := svc.Groups()
	if err != nil || len(gs) != 0 {
		t.Fatalf("导入前 Groups 应返回空且不报错: %v, %v", gs, err)
	}
}

func TestChannelsNilStore(t *testing.T) {
	channels := []config.Channel{
		{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}},
	}
	svc := NewShellService(live.New(channels), nil, nil)
	chs, err := svc.Channels("央视", 0)
	if err != nil || len(chs) != 1 || chs[0].Favorited {
		t.Fatalf("store 为 nil 时 Channels 应返回频道且 Favorited=false: %+v, %v", chs, err)
	}
}

func TestConcurrentImportAndGroups(t *testing.T) {
	svc := newTestService(t)
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:-1 group-title=\"测试\",频道A\nhttp://x/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.ImportSubscription(path) }()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.Groups() }()
	}
	wg.Wait()
}

// stubProvider 是点播 Provider 的最小测试桩。
type stubProvider struct{ key string }

func (s *stubProvider) ID() string { return s.key }
func (s *stubProvider) Home(context.Context) ([]provider.Section, error) {
	return []provider.Section{{ID: "10", Title: "电影"}}, nil
}
func (s *stubProvider) Browse(context.Context, string, int) (provider.Page, error) {
	return provider.Page{Items: []provider.Item{{ID: "1", Title: "电影A"}}}, nil
}
func (s *stubProvider) BrowseFiltered(ctx context.Context, cat string, page int, _ map[string]string) (provider.Page, error) {
	return s.Browse(ctx, cat, page)
}
func (s *stubProvider) Search(context.Context, string) ([]provider.Item, error) { return nil, nil }
func (s *stubProvider) Detail(context.Context, string) (provider.Media, error) {
	return provider.Media{ID: "1", Title: "电影A", Sources: []string{"x"}, Episodes: []provider.Episode{{ID: "1/0/0", Source: "x", Name: "第01集"}}}, nil
}
func (s *stubProvider) Resolve(context.Context, string) (player.Stream, error) {
	return player.Stream{URL: "https://x/a.m3u8", Kind: player.StreamHLS}, nil
}

type countingVodProvider struct {
	stubProvider
	homeCalls int
}

func (p *countingVodProvider) Home(context.Context) ([]provider.Section, error) {
	p.homeCalls++
	return []provider.Section{{ID: "10", Title: "电影"}}, nil
}

func TestVodPersistenceAPIs(t *testing.T) {
	svc := newTestService(t)
	if err := svc.RecordVodSearch("斗罗"); err != nil {
		t.Fatalf("RecordVodSearch: %v", err)
	}
	history, err := svc.ListVodSearchHistory()
	if err != nil || len(history) != 1 || history[0] != "斗罗" {
		t.Fatalf("ListVodSearchHistory = %#v, %v", history, err)
	}
	if err := svc.DeleteVodSearchHistory("斗罗"); err != nil {
		t.Fatalf("DeleteVodSearchHistory: %v", err)
	}
	if err := svc.AddVodFavorite("site-a", "vod-1", "影片一", "logo", "电影"); err != nil {
		t.Fatalf("AddVodFavorite: %v", err)
	}
	ok, err := svc.IsVodFavorite("site-a", "vod-1")
	if err != nil || !ok {
		t.Fatalf("IsVodFavorite = %v, %v", ok, err)
	}
	favs, err := svc.ListVodFavorites()
	if err != nil || len(favs) != 1 || favs[0].Title != "影片一" {
		t.Fatalf("ListVodFavorites = %#v, %v", favs, err)
	}
	if err := svc.RemoveVodFavorite("site-a", "vod-1"); err != nil {
		t.Fatalf("RemoveVodFavorite: %v", err)
	}
	if err := svc.RecordVodHistory("site-a", "vod-1", "影片一", "", "ep-1", "第1集", "线路"); err != nil {
		t.Fatalf("RecordVodHistory: %v", err)
	}
	// GetVodHistory：查到刚记录的条目，关键字段回显正确。
	got, err := svc.GetVodHistory("site-a", "vod-1")
	if err != nil {
		t.Fatalf("GetVodHistory: %v", err)
	}
	if got.VodID != "vod-1" || got.EpID != "ep-1" || got.Source != "线路" {
		t.Fatalf("GetVodHistory = %+v, want vod-1/ep-1/线路", got)
	}
	// 不存在的条目返回零值（VodID 为空），不报错。
	missing, err := svc.GetVodHistory("site-a", "missing")
	if err != nil || missing.VodID != "" {
		t.Fatalf("GetVodHistory(missing) = %+v, %v, want zero VodHistoryInfo", missing, err)
	}
	if err := svc.DeleteVodHistory("site-a", "vod-1"); err != nil {
		t.Fatalf("DeleteVodHistory: %v", err)
	}
	// 删除后再查同样返回零值。
	after, err := svc.GetVodHistory("site-a", "vod-1")
	if err != nil || after.VodID != "" {
		t.Fatalf("GetVodHistory after delete = %+v, %v, want zero VodHistoryInfo", after, err)
	}
}

func TestVodCategoriesUsesShortTTLCache(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	provider := &countingVodProvider{stubProvider: stubProvider{key: "s1"}}
	svc.vods["s1"] = provider
	svc.vodCategoryNow = func() time.Time { return time.Unix(100, 0) }
	if _, err := svc.VodCategories("s1"); err != nil {
		t.Fatalf("first VodCategories: %v", err)
	}
	if _, err := svc.VodCategories("s1"); err != nil {
		t.Fatalf("cached VodCategories: %v", err)
	}
	if provider.homeCalls != 1 {
		t.Fatalf("cache hit calls Home %d times", provider.homeCalls)
	}
	svc.vodCategoryNow = func() time.Time { return time.Unix(106, 0) }
	if _, err := svc.VodCategories("s1"); err != nil {
		t.Fatalf("expired VodCategories: %v", err)
	}
	if provider.homeCalls != 2 {
		t.Fatalf("expired cache calls Home %d times", provider.homeCalls)
	}
}

func TestVodSourcesAndRoutes(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods["s1"] = &stubProvider{key: "s1"}
	svc.vodNames["s1"] = "站点一"

	srcs := svc.Sources()
	if len(srcs) != 2 || srcs[0].ID != "live" || srcs[1].ID != "s1" || srcs[1].Name != "站点一" {
		t.Fatalf("Sources 错误: %+v", srcs)
	}
	secs, err := svc.VodCategories("s1")
	if err != nil || len(secs) != 1 || secs[0].Title != "电影" {
		t.Fatalf("VodCategories 错误: %+v %v", secs, err)
	}
	m, err := svc.VodDetail("s1", "1")
	if err != nil || len(m.Episodes) != 1 || m.Sources[0] != "x" {
		t.Fatalf("VodDetail 错误: %+v %v", m, err)
	}
	if _, err := svc.VodCategories("nope"); err == nil {
		t.Fatalf("未知站点应报错")
	}
}

func writeTempM3U(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadLiveLazy(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	m3u := writeTempM3U(t, "#EXTM3U\n#EXTINF:-1 group-title=\"央视\",频道1\nhttp://x/1\n")
	svc.liveSources = []config.Live{{Name: "g1", URL: m3u}}

	// 未加载时 Groups 为空
	gs, _ := svc.Groups()
	if len(gs) != 0 {
		t.Fatalf("未加载直播时 Groups 应为空: %v", gs)
	}

	n, err := svc.LoadLive()
	if err != nil || n != 1 {
		t.Fatalf("LoadLive = %d, %v", n, err)
	}
	gs, _ = svc.Groups()
	if len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("加载后 Groups = %v", gs)
	}

	// 幂等：再次 LoadLive 直接返回
	n2, err := svc.LoadLive()
	if err != nil || n2 != 1 {
		t.Fatalf("重复 LoadLive = %d, %v", n2, err)
	}
}

func TestCollectChannelsParallelOrder(t *testing.T) {
	m3u1 := writeTempM3U(t, "#EXTM3U\n#EXTINF:-1 group-title=\"央视\",频道1\nhttp://x/1\n#EXTINF:-1 group-title=\"央视\",频道2\nhttp://x/2\n")
	m3u2 := writeTempM3U(t, "#EXTM3U\n#EXTINF:-1 group-title=\"卫视\",频道3\nhttp://x/3\n")

	lives := []config.Live{
		{Name: "g1", URL: m3u1},
		{Name: "g2", Channels: []config.Channel{{Name: "内嵌", Group: "G", URLs: []string{"http://x/0"}}}},
		{Name: "g3", URL: m3u2},
	}
	chs := collectChannels(context.Background(), lives, nil)
	if len(chs) != 4 {
		t.Fatalf("频道数 = %d, 期望 4", len(chs))
	}
	var names []string
	for _, c := range chs {
		names = append(names, c.Name)
	}
	want := []string{"频道1", "频道2", "内嵌", "频道3"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("顺序错误: 得 %v, 期望 %v", names, want)
		}
	}
}

func TestRestoreSubscriptionConfig(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	cfgs := []*config.Config{{
		Sites: []config.Site{{Key: "s1", Name: "站点一", Type: config.SiteTypeCMS, API: "http://x/api.php"}},
		Lives: config.LiveList{{Name: "g1", URL: "http://x/g1.m3u"}},
	}}
	svc1 := NewShellService(nil, nil, s)
	svc1.vodCFGs = cfgs
	svc1.liveCFGs = cfgs
	svc1.saveSubscription()

	svc2 := NewShellService(nil, nil, s)
	r, err := svc2.RestoreSubscription()
	if err != nil || r.Sites != 1 || r.LiveSources != 1 {
		t.Fatalf("RestoreSubscription = %+v, %v", r, err)
	}
	if _, ok := svc2.vods["s1"]; !ok {
		t.Fatalf("恢复后应有站点 s1")
	}
	if len(svc2.liveSources) != 1 {
		t.Fatalf("恢复后应有 1 个直播源")
	}
}

func TestRestoreSubscriptionNoSnapshot(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	svc := NewShellService(nil, nil, s)
	r, err := svc.RestoreSubscription()
	if err != nil || r != (ImportResult{}) {
		t.Fatalf("无快照应返回零值且不报错: %+v, %v", r, err)
	}
}

func TestRestoreSubscriptionPlaylist(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	chans := []config.Channel{{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}}}
	svc1 := NewShellService(nil, nil, s)
	svc1.liveChannels = chans
	svc1.saveSubscription()

	svc2 := NewShellService(nil, nil, s)
	r, err := svc2.RestoreSubscription()
	if err != nil || r.Channels != 1 {
		t.Fatalf("RestoreSubscription = %+v, %v", r, err)
	}
	gs, _ := svc2.Groups()
	if len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("恢复后 Groups = %v", gs)
	}
}

func TestCollectVodSitesSpider(t *testing.T) {
	cfgs := []*config.Config{{
		Sites: []config.Site{
			{Key: "cms", Name: "CMS站", Type: config.SiteTypeCMS, API: "http://x/api.php"},
			{Key: "sp", Name: "爬虫站", Type: config.SiteTypeSpider, API: "http://x:5757"},
			{Key: "js", Name: "JS站", Type: config.SiteTypeSpider, API: "https://example.com/spider.js"},
			{Key: "jar", Name: "JAR站", Type: config.SiteTypeSpider, API: "csp_xxx"},
			{Key: "xbpq", Name: "XBPQ站", Type: config.SiteTypeSpider, API: "csp_XBPQ", Ext: "https://example.com/xbpq.json"},
		},
	}}
	vods, names, _ := collectVodSites(cfgs)
	if len(vods) != 4 {
		t.Fatalf("应有 4 个站点（cms + spider http + js + xbpq），得 %d", len(vods))
	}
	if names["sp"] != "爬虫站" {
		t.Fatalf("names = %v", names)
	}
	if _, ok := vods["jar"]; ok {
		t.Fatalf("csp_ JAR 站点不应被收集")
	}
	if _, ok := vods["js"].(*tvbox.Spider); !ok {
		t.Fatalf("js 站点应使用 tvbox.Spider，实际类型 %T", vods["js"])
	}
	if _, ok := vods["xbpq"].(*tvbox.JarProvider); !ok {
		t.Fatalf("xbpq 站点应使用 tvbox.JarProvider，实际类型 %T", vods["xbpq"])
	}
}

func TestPlaybackTokenRejectsStaleRequests(t *testing.T) {
	svc := &ShellService{}
	firstID, ok := svc.claimPlayback(1)
	if !ok || !svc.playbackCurrent(1, firstID) {
		t.Fatalf("first playback token should be current: id=%d ok=%v", firstID, ok)
	}
	stopID, ok := svc.claimPlayback(0)
	if !ok || !svc.playbackCurrent(0, stopID) {
		t.Fatalf("stop token should invalidate active playback: id=%d ok=%v", stopID, ok)
	}
	if _, ok := svc.claimPlayback(1); ok {
		t.Fatal("stale token should not reclaim playback after stop")
	}
	secondID, ok := svc.claimPlayback(2)
	if !ok || !svc.playbackCurrent(2, secondID) {
		t.Fatalf("newer playback token should be accepted: id=%d ok=%v", secondID, ok)
	}
	if _, ok := svc.claimPlayback(1); ok {
		t.Fatal("older token should not reclaim newer playback")
	}
}

func TestStopPlaybackDoesNotInvalidateNewerToken(t *testing.T) {
	svc := &ShellService{}
	if _, ok := svc.claimPlayback(1); !ok {
		t.Fatal("first playback token should be accepted")
	}
	if _, ok := svc.claimPlayback(2); !ok {
		t.Fatal("newer playback token should be accepted")
	}
	if _, invalidated := svc.invalidatePlayback(1); invalidated {
		t.Fatal("stale stop must not invalidate a newer playback token")
	}
	if svc.playbackToken != 2 {
		t.Fatalf("newer playback token was cleared: got %d", svc.playbackToken)
	}
}

func TestPlaybackEventMapping(t *testing.T) {
	cases := []struct {
		name string
		ev   player.Event
		tok  uint64
		want PlaybackEvent
	}{
		{"playing", player.Event{Kind: player.EventPlaying}, 1,
			PlaybackEvent{Token: 1, Kind: "playing"}},
		{"buffering", player.Event{Kind: player.EventBuffering}, 1,
			PlaybackEvent{Token: 1, Kind: "buffering"}},
		{"position", player.Event{Kind: player.EventPosition, Position: 8.25}, 17,
			PlaybackEvent{Token: 17, Kind: "position", Position: 8.25}},
		{"duration", player.Event{Kind: player.EventDuration, Duration: 1500.5}, 17,
			PlaybackEvent{Token: 17, Kind: "duration", Duration: 1500.5}},
		{"error", player.Event{Kind: player.EventError, Err: errors.New("boom")}, 3,
			PlaybackEvent{Token: 3, Kind: "error", Error: "boom"}},
		{"ended", player.Event{Kind: player.EventEOF}, 4,
			PlaybackEvent{Token: 4, Kind: "ended"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := playbackEventFor(tc.ev, tc.tok)
			if !ok {
				t.Fatalf("%s: 期望事件被转发", tc.name)
			}
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
	if _, ok := playbackEventFor(player.Event{Kind: player.EventKind(99)}, 1); ok {
		t.Fatal("未知事件类型不应转发给前端")
	}
}

// bridgeTestPlayer 是桥接循环测试用的可编程 player：Events() 返回注入通道。
type bridgeTestPlayer struct {
	events chan player.Event
}

func (p *bridgeTestPlayer) Load(ctx context.Context, s player.Stream) error { return nil }
func (p *bridgeTestPlayer) Play() error                                     { return nil }
func (p *bridgeTestPlayer) Pause() error                                    { return nil }
func (p *bridgeTestPlayer) Seek(float64) error                              { return nil }
func (p *bridgeTestPlayer) SetVolume(int) error                             { return nil }
func (p *bridgeTestPlayer) SelectTrack(player.TrackKind, int) error         { return nil }
func (p *bridgeTestPlayer) State() player.State                             { return player.State{} }
func (p *bridgeTestPlayer) Events() <-chan player.Event                     { return p.events }
func (p *bridgeTestPlayer) Close() error                                    { return nil }

func newBridgeTestPlayer() *bridgeTestPlayer {
	return &bridgeTestPlayer{events: make(chan player.Event, 16)}
}

func TestPlaybackBridgeFiltersTokenAndStops(t *testing.T) {
	inner := newBridgeTestPlayer()
	// 先注入事件出口再启动桥接，避免与桥接 goroutine 并发写字段。
	svc := newShellService(nil, inner, nil)

	var mu sync.Mutex
	var got []PlaybackEvent
	svc.playbackEventEmitter = func(ev PlaybackEvent) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	}
	svc.startPlaybackBridge()
	defer func() { _ = svc.ServiceShutdown() }()

	// 无会话 token 时事件应被丢弃。
	inner.events <- player.Event{Kind: player.EventPlaying}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if n := len(got); n != 0 {
		mu.Unlock()
		t.Fatalf("无 token 时收到 %d 个事件，期望 0", n)
	}
	mu.Unlock()

	// 建立会话后事件应带当前 token 发出。
	if _, ok := svc.claimPlayback(7); !ok {
		t.Fatal("claimPlayback 应接受首个 token")
	}
	inner.events <- player.Event{Kind: player.EventPosition, Position: 8.25}
	waitForBridgeEvents(t, &mu, &got, 1)
	mu.Lock()
	if len(got) != 1 || got[0] != (PlaybackEvent{Token: 7, Kind: "position", Position: 8.25}) {
		mu.Unlock()
		t.Fatalf("got %#v，期望 {Token:7 Kind:position Position:8.25}", got)
	}
	mu.Unlock()

	// shutdown 后桥接应停止：后续事件不再被转发。
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	inner.events <- player.Event{Kind: player.EventEOF}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if n := len(got); n != 1 {
		mu.Unlock()
		t.Fatalf("shutdown 后仍收到事件（共 %d）", n)
	}
	mu.Unlock()
}

func waitForBridgeEvents(t *testing.T, mu *sync.Mutex, got *[]PlaybackEvent, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(*got) >= n {
			mu.Unlock()
			return
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待桥接事件超时")
}

// TestPlaybackBridgeRestartsWithNewPlayer 覆盖 RefreshMPV 的换播放器路径：
// 启动时 mpv 缺失（player 为 nil）根本没起桥接；旧桥接即使在跑，也还堵在
// 旧播放器的 Events 通道上（该通道永不关闭）。换播放器后必须停掉旧桥接、
// 重新挂上新播放器，否则新播放器的事件永远到不了前端，播片没有进度、
// 播完不自动切集、出错不自动换源，直到重启应用才恢复。
func TestPlaybackBridgeRestartsWithNewPlayer(t *testing.T) {
	oldPlayer := newBridgeTestPlayer()
	svc := newShellService(nil, oldPlayer, nil)

	var mu sync.Mutex
	var got []PlaybackEvent
	svc.playbackEventEmitter = func(ev PlaybackEvent) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	}
	svc.startPlaybackBridge()
	defer func() { _ = svc.ServiceShutdown() }()

	if _, ok := svc.claimPlayback(7); !ok {
		t.Fatal("claimPlayback 应接受首个 token")
	}
	// 旧播放器的事件能正常转发。
	oldPlayer.events <- player.Event{Kind: player.EventPosition, Position: 1}
	waitForBridgeEvents(t, &mu, &got, 1)

	// 模拟 RefreshMPV 换播放器：停旧桥接、换 player、重新挂桥接。
	newPlayer := newBridgeTestPlayer()
	svc.stopPlaybackBridge()
	svc.player = newPlayer
	svc.startPlaybackBridge()

	// 新播放器的事件应能转发；旧播放器的事件不再被转发。
	newPlayer.events <- player.Event{Kind: player.EventPosition, Position: 8.25}
	waitForBridgeEvents(t, &mu, &got, 2)
	mu.Lock()
	if len(got) != 2 || got[1] != (PlaybackEvent{Token: 7, Kind: "position", Position: 8.25}) {
		mu.Unlock()
		t.Fatalf("换播放器后新事件未转发，got %#v", got)
	}
	mu.Unlock()
}

// preloadTestProvider 是点播预载测试用的最小 Provider。
type preloadTestProvider struct {
	stream player.Stream
	calls  int
}

func (p *preloadTestProvider) ID() string { return "preload-test" }
func (p *preloadTestProvider) Home(context.Context) ([]provider.Section, error) {
	return nil, nil
}
func (p *preloadTestProvider) Browse(context.Context, string, int) (provider.Page, error) {
	return provider.Page{}, nil
}
func (p *preloadTestProvider) BrowseFiltered(ctx context.Context, cat string, page int, _ map[string]string) (provider.Page, error) {
	return p.Browse(ctx, cat, page)
}
func (p *preloadTestProvider) Search(context.Context, string) ([]provider.Item, error) {
	return nil, nil
}
func (p *preloadTestProvider) Detail(context.Context, string) (provider.Media, error) {
	return provider.Media{}, nil
}
func (p *preloadTestProvider) Resolve(context.Context, string) (player.Stream, error) {
	p.calls++
	return p.stream, nil
}

func TestPreloadVodRegistersPreloadWithoutTouchingPlayback(t *testing.T) {
	svc := newTestService(t)
	// 用本地文件流避免点播预载测试依赖网络：它走 mpv 分支，只登记可取消的预载任务。
	pv := &preloadTestProvider{stream: player.Stream{URL: "file:///tmp/movie.mp4", Kind: player.StreamLocal}}
	svc.vods["demo"] = pv
	svc.playbackToken = 7
	svc.playbackSeq = 3

	plan, err := svc.PreloadVod("demo", "ep-2")
	if err != nil {
		t.Fatalf("PreloadVod: %v", err)
	}
	if plan.Backend != playback.BackendMPV || plan.ID == "" {
		t.Fatalf("plan = %#v，期望可释放的 mpv 预载计划", plan)
	}
	if svc.playbackToken != 7 || svc.playbackSeq != 3 {
		t.Fatalf("预载不应改动播放会话: token=%d seq=%d", svc.playbackToken, svc.playbackSeq)
	}
	if err := svc.ReleasePreload(plan.ID); err != nil {
		t.Fatalf("ReleasePreload: %v", err)
	}
	if err := svc.ReleasePreload("unknown"); err != nil {
		t.Fatalf("释放未知预载应幂等: %v", err)
	}
}

func TestPreloadVodUnknownSiteDoesNotTouchPlayback(t *testing.T) {
	svc := newTestService(t)
	svc.playbackToken = 7
	svc.playbackSeq = 3

	if _, err := svc.PreloadVod("missing", "ep-1"); err == nil {
		t.Fatal("未知站点应返回错误")
	}
	if svc.playbackToken != 7 || svc.playbackSeq != 3 {
		t.Fatalf("预载失败不应改动播放会话: token=%d seq=%d", svc.playbackToken, svc.playbackSeq)
	}
}

func TestResolveConfigsResolvesRelativeSiteURLs(t *testing.T) {
	raw := []byte(`{"sites":[{"key":"js","name":"JS 站","type":3,"api":"./lib/drpy2.min.js"}]}`)
	cfgs, err := resolveConfigs(context.Background(), "https://cfg.example.com/feed/api.json", raw)
	if err != nil {
		t.Fatalf("resolveConfigs: %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("配置数 = %d", len(cfgs))
	}
	want := "https://cfg.example.com/feed/lib/drpy2.min.js"
	if got := cfgs[0].Sites[0].API; got != want {
		t.Fatalf("站点 API = %q，期望解析成 %q", got, want)
	}
}

func TestCheckSubscriptionContentRejectsWebPage(t *testing.T) {
	html := []byte("\xef\xbb\xbf<!DOCTYPE html>\n<html><head><title>导航</title></head><body>hi</body></html>\n")
	err := checkSubscriptionContent(html, "https://www.example.com/")
	if err == nil {
		t.Fatal("网页内容应当被拒绝并给出提示")
	}
	if !strings.Contains(err.Error(), "网页") || !strings.Contains(err.Error(), "JSON 接口") {
		t.Fatalf("错误提示不够明确: %v", err)
	}

	okCases := [][]byte{
		[]byte(`{"sites":[{"key":"a","api":"http://x/api.php"}]}`),
		[]byte("#EXTM3U\n#EXTINF:-1,频道\nhttp://x/1\n"),
		[]byte("频道一,http://x/1\n"),
		[]byte("   \n\t"),
	}
	for _, raw := range okCases {
		if err := checkSubscriptionContent(raw, "https://example.com/x.json"); err != nil {
			t.Fatalf("正常内容被误拒: %v (raw=%q)", err, raw)
		}
	}
}

// 真实场景（如饭太硬 `http://www.饭太硬.cc/tv`）：URL 实际返回一张 100×100
// 的 JPEG 图片而非订阅配置。旧实现没有二进制识别，图片既不命中网页嗅探，
// 又被 isPlaylist 当作 TXT 播放列表，于是点播导入误报「直播播放列表」、
// 直播导入误报「没有可用频道」——两个互相矛盾的错误。
// 必须拦在更上层：图片 / 二进制内容给出明确提示。
func TestCheckSubscriptionContentRejectsBinary(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xc0\x00dr\x00d\x02")
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00d\x00\x00\x00d")
	bmp := []byte("BM\x00\x00\x00\x00\x00\x00\x00\x00\x36\x00\x00\x00")
	gzip := []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x00\x03binarytail")
	// 无魔数的裸二进制：靠 NUL 字节兜底识别。
	rawBinary := []byte("\x01\x02\x00garbage\x00\xff\xfe")

	for _, raw := range [][]byte{jpeg, png, bmp, gzip, rawBinary} {
		err := checkSubscriptionContent(raw, "https://example.com/xxx")
		if err == nil {
			t.Fatalf("二进制内容应当被拒绝 (head=%x)", raw[:8])
		}
		if !strings.Contains(err.Error(), "图片") && !strings.Contains(err.Error(), "二进制") {
			t.Fatalf("二进制报错应点明图片/二进制: %v", err)
		}
	}
}

func TestVodSkipMarksRoundTripAndDefaults(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/skip.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := newPlaybackSettingsService(t, st)

	if got := svc.GetVodSkipMarks("demo", "1001"); got != (VodSkipMarks{}) {
		t.Fatalf("缺失时为 %#v，期望零值", got)
	}
	want := VodSkipMarks{IntroEnd: 92.5, OutroStart: 1430}
	if err := svc.SetVodSkipMarks("demo", "1001", want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetVodSkipMarks("demo", "1001"); got != want {
		t.Fatalf("往返读到 %#v，期望 %#v", got, want)
	}
	// 不同剧集互不影响
	if got := svc2.GetVodSkipMarks("demo", "1002"); got != (VodSkipMarks{}) {
		t.Fatalf("其它剧集不应共享标记: %#v", got)
	}
}

func TestVodSkipMarksInvalidJSONFallsBackToZero(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/skip-invalid.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := newPlaybackSettingsService(t, st)
	if err := st.SetKV(vodSkipKey("demo", "1001"), "not-json"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetVodSkipMarks("demo", "1001"); got != (VodSkipMarks{}) {
		t.Fatalf("非法值应回退零值，得 %#v", got)
	}
}

func TestVodSkipMarksStoreUnavailable(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/skip-closed.db")
	if err != nil {
		t.Fatal(err)
	}
	svc := newPlaybackSettingsService(t, st)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetVodSkipMarks("demo", "1001"); got != (VodSkipMarks{}) {
		t.Fatalf("store 不可用应返回零值，得 %#v", got)
	}
	if err := svc.SetVodSkipMarks("demo", "1001", VodSkipMarks{IntroEnd: 1}); err == nil {
		t.Fatal("store 不可用时写入应报错")
	}
}

// 回归：SourceURL 曾用 json:"-" 排除在持久化之外，RestoreSubscription 又只做纯内存重建，
// 于是快照里带相对 api/ext 的站点永远不会被解析成绝对地址，重启后才在运行时
// 报「爬虫地址不是完整 URL（./lib/douyu.js）：配置里的相对路径未解析成功」。
func TestRestoreSubscriptionResolvesRelativeURLs(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	// 直接构造「带相对 api/ext 的终端配置」，模拟旧版把它原样存进快照。
	cfgs := []*config.Config{{
		SourceURL: "https://gh-proxy.com/raw.githubusercontent.com/noimank/tvbox/main/tvbox.json",
		Sites: []config.Site{
			{Key: "drpy", Name: "drpy站", Type: config.SiteTypeSpider, API: "./lib/drpy2.min.js", Ext: "./js/聚合短剧.js"},
			{Key: "cms", Name: "CMS站", Type: config.SiteTypeCMS, API: "http://x/api.php"},
		},
		Lives: config.LiveList{{Name: "g1", URL: "./list.txt"}},
	}}

	svc1 := NewShellService(nil, nil, s)
	svc1.vodCFGs = cfgs
	svc1.liveCFGs = cfgs
	svc1.saveSubscription()

	svc2 := NewShellService(nil, nil, s)
	if _, err := svc2.RestoreSubscription(); err != nil {
		t.Fatalf("RestoreSubscription: %v", err)
	}

	sp, ok := svc2.vods["drpy"].(*tvbox.Spider)
	if !ok {
		t.Fatalf("恢复后 drpy 应为 Spider，实际 %T", svc2.vods["drpy"])
	}
	const wantScript = "https://gh-proxy.com/raw.githubusercontent.com/noimank/tvbox/main/js/聚合短剧.js"
	if got := sp.ScriptURL(); got != wantScript {
		t.Fatalf("恢复后脚本地址 = %q，期望 %q", got, wantScript)
	}
	if len(svc2.liveSources) != 1 ||
		svc2.liveSources[0].URL != "https://gh-proxy.com/raw.githubusercontent.com/noimank/tvbox/main/list.txt" {
		t.Fatalf("恢复后直播源 = %+v", svc2.liveSources)
	}
}

// SourceURL 必须参与持久化，否则恢复时无从解析相对路径。
func TestPersistedSubscriptionKeepsSourceURL(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	svc := NewShellService(nil, nil, s)
	svc.vodCFGs = []*config.Config{{
		SourceURL: "https://a.example.com/sub.json",
		Sites:     []config.Site{{Key: "s1", Type: config.SiteTypeCMS, API: "http://x/api.php"}},
	}}
	svc.saveSubscription()

	b, ok, err := s.GetKV(subscriptionKey)
	if err != nil || !ok {
		t.Fatalf("读取快照失败: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(b, "https://a.example.com/sub.json") {
		t.Fatalf("快照应保留 SourceURL，实际内容: %s", b)
	}
}

// 证书豁免列表要能持久化并重启恢复；主机名需归一化（去协议/端口/大小写）。
func TestInsecureTLSHostsPersistAndNormalize(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	defer httpx.SetInsecureHosts(nil)

	svc := NewShellService(nil, nil, s)
	hosts, err := svc.SetInsecureTLSHost("https://HaiwaiKan.com:443/api.php", true)
	if err != nil {
		t.Fatalf("SetInsecureTLSHost: %v", err)
	}
	if len(hosts) != 1 || hosts[0] != "haiwaikan.com" {
		t.Fatalf("归一化后 = %v", hosts)
	}

	svc2 := NewShellService(nil, nil, s)
	if got := svc2.ListInsecureTLSHosts(); len(got) != 1 || got[0] != "haiwaikan.com" {
		t.Fatalf("重启后恢复 = %v", got)
	}

	hosts, err = svc2.SetInsecureTLSHost("haiwaikan.com", false)
	if err != nil {
		t.Fatalf("取消豁免: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("取消后应为空，得 %v", hosts)
	}
}

// panicProvider 的 Search 必然 panic，用于验证全站搜索的站点级容错：
// 某个站点脚本/Provider 崩溃时，只跳过该站点，不能让整个搜索（和应用）闪退。
type panicProvider struct{ provider.Provider }

func (panicProvider) Search(context.Context, string) ([]provider.Item, error) {
	panic("boom: 模拟 goja 对象转字符串崩溃")
}

func (panicProvider) BrowseFiltered(context.Context, string, int, map[string]string) (provider.Page, error) {
	return provider.Page{}, nil
}

type stubSearchProvider struct {
	provider.Provider
	items []provider.Item
}

func (p stubSearchProvider) Search(context.Context, string) ([]provider.Item, error) {
	return p.items, nil
}

func (p stubSearchProvider) BrowseFiltered(context.Context, string, int, map[string]string) (provider.Page, error) {
	return provider.Page{}, nil
}

func TestVodSearchAllSurvivesPanickingSite(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{
		"正常": stubSearchProvider{items: []provider.Item{{ID: "1", Title: "结果"}}},
		"崩溃": panicProvider{},
	}
	items, err := svc.VodSearchAll("测试")
	if err != nil {
		t.Fatalf("全站搜索应返回（崩溃站点被跳过），got err=%v", err)
	}
	if len(items) != 1 || items[0].Title != "结果" {
		t.Fatalf("崩溃站点应被跳过且正常站点结果保留，got %+v", items)
	}
}

// errSearchProvider 固定返回错误的站点，用于验证全站搜索的失败可见性：
// 站点自身失败时应记日志（而不是静默吞掉），正常站点结果不受影响。
type errSearchProvider struct {
	provider.Provider
	err error
}

func (p errSearchProvider) Search(context.Context, string) ([]provider.Item, error) {
	return nil, p.err
}

func (p errSearchProvider) BrowseFiltered(context.Context, string, int, map[string]string) (provider.Page, error) {
	return provider.Page{}, nil
}

func TestVodSearchAllLogsFailingSite(t *testing.T) {
	var buf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(oldOut)

	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{
		"坏站点": errSearchProvider{err: errors.New("规则解析失败")},
		"正常":  stubSearchProvider{items: []provider.Item{{ID: "1", Title: "结果"}}},
	}
	items, err := svc.VodSearchAll("q")
	if err != nil {
		t.Fatalf("失败站点不应让整体搜索失败，got %v", err)
	}
	if len(items) != 1 || items[0].Title != "结果" {
		t.Fatalf("正常站点结果应保留，got %+v", items)
	}
	if !strings.Contains(buf.String(), "坏站点") || !strings.Contains(buf.String(), "失败") {
		t.Fatalf("失败站点应记日志，got %q", buf.String())
	}
}

// slowSearchProvider 故意拖住 Search；配合站点级超时验证并发搜索不会
// 被慢站点一直占住线程（超时的站点按失败跳过，整体照常返回）。
type slowSearchProvider struct {
	provider.Provider
	delay time.Duration
}

func (p slowSearchProvider) Search(ctx context.Context, _ string) ([]provider.Item, error) {
	select {
	case <-time.After(p.delay):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestVodSearchAllBindsPerSiteTimeout(t *testing.T) {
	oldTimeout := siteSearchTimeout
	siteSearchTimeout = 100 * time.Millisecond
	defer func() { siteSearchTimeout = oldTimeout }()

	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{
		"慢站点": slowSearchProvider{delay: time.Second},
		"正常":  stubSearchProvider{items: []provider.Item{{ID: "1", Title: "结果"}}},
	}
	start := time.Now()
	items, err := svc.VodSearchAll("q")
	if err != nil {
		t.Fatalf("慢站点超时后整体应成功，got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("慢站点应被跳过、正常站点结果保留，got %d", len(items))
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("单站点应被 %s 超时截断，实际耗时 %s", siteSearchTimeout, elapsed)
	}

	// 超时的站点进入短时跳过名单：紧接着的第二次搜索不再为它白等。
	start = time.Now()
	if _, err := svc.VodSearchAll("q"); err != nil {
		t.Fatalf("第二次全站搜索应成功，got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("超时站点应在后续搜索中短时跳过，实际耗时 %s", elapsed)
	}
}

// 回归测试（D1 同类症状）：全站搜索一个结果都没有、且所有站点都失败或
// 被跳过时，不能再静默返回空列表——前端会把它当成“搜不到这部片”显示
// “暂无搜索结果”，用户看不到任何错误、也不知道可以重试；应返回明确报错。
func TestVodSearchAllErrorsWhenAllSitesFail(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{
		"坏站点": errSearchProvider{err: errors.New("规则解析失败")},
		"崩溃":  panicProvider{},
	}
	items, err := svc.VodSearchAll("小城良方")
	if err == nil {
		t.Fatal("所有站点都失败时应返回报错，而不是静默空列表")
	}
	if !strings.Contains(err.Error(), "失败") {
		t.Fatalf("报错应说明站点失败，实际: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("所有站点失败时不应有结果，got %d", len(items))
	}
}

// 回归测试：全站搜索一个结果都没有、且所有站点都因上次超时被跳过时，
// 同样不能静默返回空列表；应报错说明站点被跳过、提示稍后重试。
func TestVodSearchAllErrorsWhenAllSitesSkipped(t *testing.T) {
	oldTimeout := siteSearchTimeout
	siteSearchTimeout = 50 * time.Millisecond
	defer func() { siteSearchTimeout = oldTimeout }()

	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{"慢站点": slowSearchProvider{delay: time.Second}}
	if _, err := svc.VodSearchAll("q"); err == nil {
		t.Fatal("超时站点失败时应返回报错，而不是静默空列表")
	}
	// 紧接着的第二次搜索：慢站点在跳过名单里，应直接报错、不再白等。
	start := time.Now()
	_, err := svc.VodSearchAll("q")
	if err == nil {
		t.Fatal("所有站点被跳过时应返回报错，而不是静默空列表")
	}
	if !strings.Contains(err.Error(), "跳过") {
		t.Fatalf("报错应说明站点被跳过，实际: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("超时站点应在后续搜索中短时跳过，实际耗时 %s", elapsed)
	}
}

// 743 个站点里大半是死站点，每次启动第一次搜索都要重罚一遍；名单必须落盘。
func TestSearchDeadSitesSurviveRestart(t *testing.T) {
	oldTimeout := siteSearchTimeout
	siteSearchTimeout = 50 * time.Millisecond
	defer func() { siteSearchTimeout = oldTimeout }()

	st, err := store.Open(filepath.Join(t.TempDir(), "dead.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := NewShellService(nil, nil, st)
	svc.vods = map[string]provider.Provider{"慢站点": slowSearchProvider{delay: time.Second}}
	// 唯一站点超时失败、0 结果：应报错（而不是静默空列表），名单同时落盘。
	if _, err := svc.VodSearchAll("q"); err == nil {
		t.Fatal("唯一站点超时失败时应返回报错")
	}

	// 模拟重启：新服务实例，同一份 store。
	rebooted := NewShellService(nil, nil, st)
	rebooted.vods = map[string]provider.Provider{"慢站点": slowSearchProvider{delay: time.Second}}
	start := time.Now()
	// 重启后第一次搜索：死站点直接跳过，0 结果、应报错说明被跳过。
	if _, err := rebooted.VodSearchAll("q"); err == nil {
		t.Fatal("死站点被跳过、0 结果时应返回报错")
	}
	if elapsed := time.Since(start); elapsed > 30*time.Millisecond {
		t.Fatalf("死站点名单应在重启后保留并直接跳过，实际耗时 %s", elapsed)
	}
}

// 线程数缓存在内存里，不在每次搜索时回查数据库：回查一旦撞上 SQLITE_BUSY
// 就会被静默降级成 1，用户把线程调到 16 也照样串行（真实踩过）。
func TestSearchThreadsServedFromMemory(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewShellService(nil, nil, st)

	if err := svc.SetSearchThreads(16); err != nil {
		t.Fatal(err)
	}
	if got := svc.SearchThreads(); got != 16 {
		t.Fatalf("SearchThreads() = %d, want 16", got)
	}
	// 上限 16；超过按上限收敛。
	if err := svc.SetSearchThreads(128); err != nil {
		t.Fatal(err)
	}
	if got := svc.SearchThreads(); got != 16 {
		t.Fatalf("SearchThreads() = %d, want 16（上限）", got)
	}
	// 库读不出来也不能把内存里的设置打回 1。
	st.Close()
	if got := svc.SearchThreads(); got != 16 {
		t.Fatalf("数据库不可用后 SearchThreads() = %d, want 16", got)
	}
}

// 透传筛选值到 Provider.BrowseFiltered（Task 7 验收）。
type filterProbeProvider struct {
	provider.Provider
	got map[string]string
}

func (p *filterProbeProvider) BrowseFiltered(ctx context.Context, _ string, _ int, filters map[string]string) (provider.Page, error) {
	p.got = filters
	return provider.Page{}, nil
}

func (p *filterProbeProvider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}

func TestVodListFilteredRoutesFilters(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	pr := &filterProbeProvider{}
	svc.vods = map[string]provider.Provider{"f": pr}
	if _, err := svc.VodListFiltered("f", "1", 1, map[string]string{"area": "欧美"}); err != nil {
		t.Fatal(err)
	}
	if pr.got["area"] != "欧美" {
		t.Fatalf("VodListFiltered 未透传筛选值: %+v", pr.got)
	}
}

// 阻塞型探针：等 ctx 取消才返回（模拟聚合源串行拉多个子站、规则脚本长
// 时间执行等永远不返回的源站）；VodListFiltered 必须在整体超时内打断它。
type blockingProbeProvider struct {
	provider.Provider
}

func (p *blockingProbeProvider) BrowseFiltered(ctx context.Context, _ string, _ int, _ map[string]string) (provider.Page, error) {
	<-ctx.Done()
	return provider.Page{}, ctx.Err()
}

func (p *blockingProbeProvider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}

func (p *blockingProbeProvider) Detail(ctx context.Context, _ string) (provider.Media, error) {
	<-ctx.Done()
	return provider.Media{}, ctx.Err()
}

func (p *blockingProbeProvider) Resolve(ctx context.Context, _ string) (player.Stream, error) {
	<-ctx.Done()
	return player.Stream{}, ctx.Err()
}

func (p *blockingProbeProvider) Search(ctx context.Context, _ string) ([]provider.Item, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// 回归测试（第 1 项）：点播列表加载必须有整体超时。源站一直不返回时，
// 此前 VodListFiltered 用 Background、无任何上限，点播页会一直空白
// （无加载态、无报错、无重试）；修好后应在超时内返回并说明超时。
func TestVodListFilteredTimesOut(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{"slow": &blockingProbeProvider{}}
	old := vodListTimeout
	vodListTimeout = 50 * time.Millisecond
	defer func() { vodListTimeout = old }()
	start := time.Now()
	_, err := svc.VodListFiltered("slow", "1", 1, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("永远不返回的源站应当超时报错")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("超时报错应说明超时，实际: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("VodListFiltered 超时耗时 %v，远超整体超时", elapsed)
	}
}

// 回归测试：点播详情加载必须有整体超时。源站一直不返回时，此前 VodDetail
// 用 Background、无任何上限，详情页会一直转圈、看不到选集/线路（D1 同类症状）；
// 修好后应在超时内返回并说明超时。
func TestVodDetailTimesOut(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{"slow": &blockingProbeProvider{}}
	old := vodDetailTimeout
	vodDetailTimeout = 50 * time.Millisecond
	defer func() { vodDetailTimeout = old }()
	start := time.Now()
	_, err := svc.VodDetail("slow", "1")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("永远不返回的源站应当超时报错")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("超时报错应说明超时，实际: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("VodDetail 超时耗时 %v，远超整体超时", elapsed)
	}
}

// 回归测试：点播播放准备里的剧集解析必须有整体超时。源站一直不返回时，
// 此前 prepareVod 用 Background 解析，点播会一直播不出来（无报错、无重试）。
func TestPrepareVodResolveTimesOut(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{"slow": &blockingProbeProvider{}}
	old := vodDetailTimeout
	vodDetailTimeout = 50 * time.Millisecond
	defer func() { vodDetailTimeout = old }()
	start := time.Now()
	_, err := svc.PrepareVod("slow", "1/0/0")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("永远不返回的源站应当超时报错")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("超时报错应说明超时，实际: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("PrepareVod 解析超时耗时 %v，远超整体超时", elapsed)
	}
}

// 回归测试：单站搜索必须有整体超时。源站一直不返回时，此前 VodSearch
// 用 Background、无任何上限，点播搜索页会一直转圈（无报错、无重试）；
// 修好后应在超时内返回并说明超时。
func TestVodSearchTimesOut(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods = map[string]provider.Provider{"slow": &blockingProbeProvider{}}
	old := siteSearchTimeout
	siteSearchTimeout = 50 * time.Millisecond
	defer func() { siteSearchTimeout = old }()
	start := time.Now()
	_, err := svc.VodSearch("slow", "小城良方")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("永远不返回的源站应当超时报错")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("超时报错应说明超时，实际: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("VodSearch 超时耗时 %v，远超整体超时", elapsed)
	}
}

// 检查更新走网页重定向而不是 API：未登录的 GitHub API 每小时只有 60 次，
// 被限流时只会 403（用户现场就是这样）。
func TestLatestReleaseByRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v1.2.3", http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/v1.2.3", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>release</html>"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tag, html, found, err := latestReleaseByRedirect(srv.Client(), srv.URL+"/releases/latest")
	if err != nil || !found {
		t.Fatalf("tag=%q found=%v err=%v", tag, found, err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("tag=%q, want v1.2.3", tag)
	}
	if !strings.Contains(html, "/releases/tag/v1.2.3") {
		t.Fatalf("html=%q", html)
	}
}

func TestLatestReleaseByRedirectNoRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	})
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html></html>"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, _, found, err := latestReleaseByRedirect(srv.Client(), srv.URL+"/releases/latest")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if found {
		t.Fatal("没有 release 时不该报有新版本")
	}
}

// 限流要给能看懂的提示，而不是干巴巴的 HTTP 403。
func TestLatestReleaseByAPIRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()

	_, _, _, err := latestReleaseByAPI(srv.Client(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "限流") {
		t.Fatalf("err=%v, 应提示限流", err)
	}
}

// 检查更新必须查本 fork 的 releases：上游 teaGod-s/UnBox 已发 v1.0.0，
// 本 fork 最新是 v0.9.10，查上游会让每个 fork 构建都误报「发现新版本 v1.0.0」。
func TestUpdateRepoPointsAtFork(t *testing.T) {
	if updateRepo != "ciweihaha/UnBox" {
		t.Fatalf("updateRepo=%q, want ciweihaha/UnBox", updateRepo)
	}
	if !strings.Contains(updatePageURL, "github.com/ciweihaha/UnBox/releases/latest") {
		t.Fatalf("updatePageURL=%q", updatePageURL)
	}
	if !strings.Contains(updateURL, "api.github.com/repos/ciweihaha/UnBox/releases/latest") {
		t.Fatalf("updateURL=%q", updateURL)
	}
}

// compareVersion 按语义化版本比较：缺失的段按 0 算，预发布版小于正式版。
func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.9.10", "0.9.9", 1},
		{"0.9.10", "v0.9.10", 0},
		{"1.0", "1.0.0", 0},
		{"1.0.0", "1.0", 0},
		{"1.0.0-beta", "1.0.0", -1},
		{"1.0.0", "1.0.0-beta", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"1.0.0+build", "1.0.0", 0},
		{"2.0", "10.0", -1},
	}
	for _, c := range cases {
		if got := compareVersion(c.a, c.b); got != c.want {
			t.Errorf("compareVersion(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestShellServiceHTTPClientUsesEnvProxy：点播源目录拉取走外网，必须经统一
// 代理选择，否则只改系统代理（v2rayN）的桌面启动方式下直连慢路。
// Linux 上裸客户端的 DefaultTransport 本来就认环境变量，行为测不出差别，
// 真正的差别在 Windows 系统代理，所以这里钉接线：传输层的 Proxy 必须是
// 统一选择函数 httpx.ProxyForRequest。
func TestShellServiceHTTPClientUsesEnvProxy(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	tr, ok := svc.httpClient.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("ShellService 没有显式传输层（Transport = %v）", svc.httpClient.Transport)
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(httpx.ProxyForRequest).Pointer() {
		t.Fatalf("ShellService 没走统一代理选择函数 httpx.ProxyForRequest")
	}
}

// TestCheckUpdateUsesEnvProxy：检查更新走外网（github.com），必须经统一
// 代理选择，否则只改系统代理的桌面启动方式下请求直连慢路、10 秒超时失败。
// 用本地代理桩 + 本地目标站验证整条 CheckUpdate 路径确实经过代理。
func TestCheckUpdateUsesEnvProxy(t *testing.T) {
	tagSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>release</html>"))
	}))
	defer tagSrv.Close()
	var viaProxy atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Add(1)
		http.Redirect(w, r, tagSrv.URL+"/releases/tag/v9.9.9", http.StatusFound)
	}))
	defer proxy.Close()

	oldPage, oldAPI := updatePageURL, updateURL
	updatePageURL = "http://update-test.invalid/releases/latest"
	updateURL = "http://update-test.invalid/api"
	defer func() { updatePageURL, updateURL = oldPage, oldAPI }()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	svc := NewShellService(nil, nil, nil)
	info, err := svc.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate 报错: %v", err)
	}
	if info.LatestVersion != "v9.9.9" {
		t.Fatalf("LatestVersion = %q，想要 v9.9.9", info.LatestVersion)
	}
	if viaProxy.Load() != 1 {
		t.Fatalf("经代理的请求次数 = %d，想要 1", viaProxy.Load())
	}
}
