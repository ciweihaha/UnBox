package shell

import (
	"context"
	"errors"
	"testing"

	"github.com/unbox/unbox/internal/provider"
)

func TestSimilarCoreTitle(t *testing.T) {
	cases := map[string]string{
		"双峰 第二季":                "双峰",
		"双峰第二季":                 "双峰",
		"庆余年 第2季":               "庆余年",
		"庆余年 第 2 季":             "庆余年",
		"小猪佩奇全集":                "小猪佩奇",
		"小猪佩奇 合集":               "小猪佩奇",
		"荒野求生 第01集":             "荒野求生",
		"流浪地球2":                 "流浪地球2", // 末尾数字是片名的一部分，不能去掉
		"流浪地球 (2019)":           "流浪地球",
		"流浪地球（2019）":            "流浪地球",
		"哪吒之魔童闹海·2025":          "哪吒之魔童闹海",
		"Breaking Bad Season 2": "Breaking Bad",
		"Breaking Bad S02":      "Breaking Bad",
		"双峰":                    "双峰",
		"  双峰 第二季  ":            "双峰",
	}
	for in, want := range cases {
		if got := similarCoreTitle(in); got != want {
			t.Errorf("similarCoreTitle(%q) = %q，想要 %q", in, got, want)
		}
	}
}

func TestFilterSimilarItems(t *testing.T) {
	items := []VodItem{
		{ID: "1", Title: "双峰", Site: "a"},
		{ID: "2", Title: "双峰 第二季", Site: "b"},
		{ID: "3", Title: "双峰：回归", Site: "a"},  // 同名变体，不要
		{ID: "4", Title: "双峰", Site: "b"},     // 同名不同站，去重
		{ID: "5", Title: "  双峰  ", Site: "c"}, // 空白归一化后同名，去重
		{ID: "6", Title: "双峰 第一季", Site: "a"}, // 同名变体（另一季），不要
		{ID: "7", Title: "大 Twin Peaks", Site: "a"},
		{ID: "8", Title: "真探", Site: "a"},
	}
	got := filterSimilarItems(items, "双峰 第二季", "2", 10)
	// 当前这部（ID 2）去掉；同名与同名变体都去掉；只留真正不同片名的两条。
	if len(got) != 2 {
		t.Fatalf("过滤后 = %d 条，想要 2 条：%+v", len(got), got)
	}
	for _, it := range got {
		if it.ID == "2" {
			t.Fatalf("当前这部（ID 2）不应出现在同类里：%+v", got)
		}
		if it.Title == "双峰：回归" || it.Title == "双峰 第一季" {
			t.Fatalf("同名变体不应出现在同类里：%+v", got)
		}
	}
	// 上限截断。
	if got := filterSimilarItems(items, "双峰", "", 1); len(got) != 1 {
		t.Fatalf("上限 1 应截断为 1 条，实际 %d 条", len(got))
	}
}

// isSameTitleVariant：4K 版、其它语言版、另一季、副标题都算同名变体。
func TestIsSameTitleVariant(t *testing.T) {
	cases := []struct {
		candidate, core string
		want            bool
	}{
		{"小城良方4K", "小城良方", true},
		{"小城良方阿拉伯语", "小城良方", true},
		{"小城良方 斯洛文尼亚语", "小城良方", true},
		{"小城良方", "小城良方", true},
		{"双峰 第一季", "双峰", true},
		{"双峰：回归", "双峰", true},
		{"小城之春", "小城良方", false},
		{"大 Twin Peaks", "双峰", false},
		{"真探", "双峰", false},
	}
	for _, c := range cases {
		if got := isSameTitleVariant(c.candidate, c.core); got != c.want {
			t.Errorf("isSameTitleVariant(%q, %q) = %v，想要 %v", c.candidate, c.core, got, c.want)
		}
	}
}

func TestSimilarCategoryKeyword(t *testing.T) {
	cases := map[[2]string]string{
		{"内地剧", "中国大陆"}: "电视",
		{"欧美剧", "欧美"}:   "电视",
		{"电影", "欧美"}:    "电影",
		{"综艺", ""}:      "综艺",
		{"动漫", "日本"}:    "动漫",
		{"动画", "日本"}:    "动漫",
		{"纪录片", ""}:     "纪录",
		{"", ""}:        "",
	}
	for in, want := range cases {
		if got := similarCategoryKeyword(in[0], in[1]); got != want {
			t.Errorf("similarCategoryKeyword(%q, %q) = %q，想要 %q", in[0], in[1], got, want)
		}
	}
}

func TestSimilarSectionFor(t *testing.T) {
	sections := []Section{
		{ID: "1", Title: "电影"},
		{ID: "2", Title: "电视剧"},
		{ID: "3", Title: "综艺"},
	}
	if sc, ok := similarSectionFor(sections, "电视"); !ok || sc.ID != "2" {
		t.Fatalf("电视类应匹配到电视剧栏目，得到 %+v, %v", sc, ok)
	}
	if sc, ok := similarSectionFor(sections, "电影"); !ok || sc.ID != "1" {
		t.Fatalf("电影类应匹配到电影栏目，得到 %+v, %v", sc, ok)
	}
	if _, ok := similarSectionFor(sections, "纪录"); ok {
		t.Fatal("没有纪录栏目时不应匹配到")
	}
}

// queryCaptureProvider 记下收到的搜索词：验证全网找同类搜的是核心词，
// 而不是带季/集后缀的完整标题。
type queryCaptureProvider struct {
	provider.Provider
	queries []string
	items   []provider.Item
}

func (p *queryCaptureProvider) Search(_ context.Context, q string) ([]provider.Item, error) {
	p.queries = append(p.queries, q)
	return p.items, nil
}

func (p *queryCaptureProvider) BrowseFiltered(context.Context, string, int, map[string]string) (provider.Page, error) {
	return provider.Page{}, nil
}

func TestVodSimilarSearchesWholeNetwork(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	capA := &queryCaptureProvider{items: []provider.Item{{ID: "a1", Title: "双峰 第一季"}}}
	svc.vods = map[string]provider.Provider{
		"站A": capA,
		"站B": stubSearchProvider{items: []provider.Item{{ID: "b1", Title: "大 Twin Peaks"}, {ID: "b2", Title: "双峰 第二季"}}},
		"坏站": errSearchProvider{err: errors.New("规则解析失败")},
	}
	// 站点传空：只走全网搜那路（不浏览栏目）。
	items, err := svc.VodSimilar("双峰 第二季", "b2", "", "", "")
	if err != nil {
		t.Fatalf("VodSimilar: %v", err)
	}
	// 搜的是核心词「双峰」，不是完整标题。
	if len(capA.queries) != 1 || capA.queries[0] != "双峰" {
		t.Fatalf("搜索词 = %v，想要 [双峰]", capA.queries)
	}
	// 同名变体（双峰 第一季、双峰 第二季）都不要，只留真正不同片名的那条；坏站被跳过。
	if len(items) != 1 || items[0].Title != "大 Twin Peaks" {
		t.Fatalf("同类 = %+v，想要 1 条「大 Twin Peaks」", items)
	}
}

// stubBrowseProvider 实现 Home + Browse：验证同类主路（栏目浏览）。
type stubBrowseProvider struct {
	provider.Provider
	sections []provider.Section
	pages    map[int][]provider.Item
	browsed  []string
}

func (p *stubBrowseProvider) Home(context.Context) ([]provider.Section, error) {
	return p.sections, nil
}

func (p *stubBrowseProvider) Browse(_ context.Context, cat string, page int) (provider.Page, error) {
	p.browsed = append(p.browsed, cat)
	return provider.Page{Items: p.pages[page], Page: page, HasMore: page < len(p.pages)}, nil
}

func (p *stubBrowseProvider) Search(_ context.Context, _ string) ([]provider.Item, error) {
	return nil, nil
}

func TestVodSimilarBrowsesCategoryFirst(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	browse := &stubBrowseProvider{
		sections: []provider.Section{{ID: "2", Title: "电视剧"}, {ID: "1", Title: "电影"}},
		pages: map[int][]provider.Item{
			1: {{ID: "c1", Title: "小城良方"}, {ID: "c2", Title: "小城之春"}, {ID: "c3", Title: "漫长的季节"}},
		},
	}
	svc.vods = map[string]provider.Provider{"本站": browse}
	items, err := svc.VodSimilar("小城良方", "c1", "本站", "内地剧", "中国大陆")
	if err != nil {
		t.Fatalf("VodSimilar: %v", err)
	}
	// 浏览的是电视剧栏目（ID 2）；当前这部去掉；不同片名的两条留下。
	if len(browse.browsed) != 1 || browse.browsed[0] != "2" {
		t.Fatalf("浏览栏目 = %v，想要 [2]", browse.browsed)
	}
	if len(items) != 2 {
		t.Fatalf("同类 = %d 条，想要 2 条：%+v", len(items), items)
	}
	for _, it := range items {
		if it.Title == "小城良方" {
			t.Fatalf("当前这部不应出现在同类里：%+v", items)
		}
	}
}
