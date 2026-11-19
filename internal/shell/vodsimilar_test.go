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
		{ID: "3", Title: "双峰：回归", Site: "a"},
		{ID: "4", Title: "双峰", Site: "b"},     // 同名不同站，去重
		{ID: "5", Title: "  双峰  ", Site: "c"}, // 空白归一化后同名，去重
		{ID: "6", Title: "大 Twin Peaks", Site: "a"},
	}
	got := filterSimilarItems(items, "双峰 第二季", "2", 10)
	// 当前这部（ID 2）去掉；同名只留一个；其余保留。
	if len(got) != 3 {
		t.Fatalf("过滤后 = %d 条，想要 3 条：%+v", len(got), got)
	}
	for _, it := range got {
		if it.ID == "2" {
			t.Fatalf("当前这部（ID 2）不应出现在同类里：%+v", got)
		}
	}
	// 上限截断。
	if got := filterSimilarItems(items, "双峰", "", 2); len(got) != 2 {
		t.Fatalf("上限 2 应截断为 2 条，实际 %d 条", len(got))
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
		"站B": stubSearchProvider{items: []provider.Item{{ID: "b1", Title: "双峰：回归"}, {ID: "b2", Title: "双峰 第二季"}}},
		"坏站": errSearchProvider{err: errors.New("规则解析失败")},
	}
	items, err := svc.VodSimilar("双峰 第二季", "b2")
	if err != nil {
		t.Fatalf("VodSimilar: %v", err)
	}
	// 搜的是核心词「双峰」，不是完整标题。
	if len(capA.queries) != 1 || capA.queries[0] != "双峰" {
		t.Fatalf("搜索词 = %v，想要 [双峰]", capA.queries)
	}
	// 当前这部（双峰 第二季）去掉；双峰 第一季、双峰：回归留下；坏站被跳过。
	if len(items) != 2 {
		t.Fatalf("同类 = %d 条，想要 2 条：%+v", len(items), items)
	}
	for _, it := range items {
		if it.Title == "双峰 第二季" {
			t.Fatalf("当前这部不应出现在同类里：%+v", items)
		}
	}
}
