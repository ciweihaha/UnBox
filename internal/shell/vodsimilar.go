package shell

// 「更多类似内容」改全网搜不同片名的同类，不走 VodSearchAll
//（那条路会取消正在进行的用户搜索、还推搜索进度事件；详情页的类似内容是
// 后台静默查询）。不要 Key：只拿标题/季/集等现成信息去各站搜同类词汇总；
// 搜不到就返回空，前端显示空状态，不硬推不相关的。
//
// v0.9.17 起同类必须是「不同片名」：之前按核心片名全网搜，5 张卡全是
// 同一标题的不同版本/语言（小城良方 4K/阿拉伯语/斯洛文尼亚语版），
// 用户要的是不同片名的同类推荐。改法：
//  1. 主路：按类型/地区在当前站点浏览同类栏目（电影/电视剧/综艺/动漫），
//     取不同片名；
//  2. 补路：核心片名全网搜，但过滤更严——标题含核心片名的同名变体
//     （第 X 季、4K 版、其它语言版）一律不要，只留真正不同片名的结果；
//  3. 两路合并去重，不够也不硬凑。

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/unbox/unbox/internal/provider"
)

// similarMaxResults 全网找同类最多返回的条数。
const similarMaxResults = 12

// similarBrowseTimeout 同类栏目浏览的单次上限：详情页静默查询，别拖太久。
const similarBrowseTimeout = 10 * time.Second

var (
	// 标题尾部的季/部/集标记：第 2 季、第2部、第01集、第二季。
	similarSeasonRe = regexp.MustCompile(`[\s·]*第\s*[0-9一二三四五六七八九十百]+\s*[季部集]\s*$`)
	similarBareJiRe = regexp.MustCompile(`[\s·]*[一二三四五六七八九十]+\s*季\s*$`)
	// 标题尾部的年份：(2019)、（2019）、·2025、 2025。
	similarYearRe = regexp.MustCompile(`[\s·]*[(（]?\d{4}[)）]?\s*$`)
	// 英文季标记：Season 2、S02。
	similarSeasonENRe = regexp.MustCompile(`(?i)[\s·]*Season\s*\d+\s*$`)
	similarSRe        = regexp.MustCompile(`(?i)[\s·]*S\d+\s*$`)
	// 全集/合集后缀。
	similarCollectionRe = regexp.MustCompile(`[\s·]*(全集|合集)\s*$`)
)

// similarCoreTitle 从详情标题里抽核心词：去掉季/部/集、年份、全集合集等
// 后缀，剩下的词拿去全网搜同类。末尾的数字是片名的一部分（如《流浪地球2》），
// 不能去掉； stripping 后为空就返回原标题。
func similarCoreTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	core := trimmed
	res := []*regexp.Regexp{similarSeasonRe, similarBareJiRe, similarYearRe, similarSeasonENRe, similarSRe, similarCollectionRe}
	for {
		changed := false
		for _, re := range res {
			if next := strings.TrimSpace(re.ReplaceAllString(core, "")); next != core {
				core = next
				changed = true
			}
		}
		if !changed || core == "" {
			break
		}
	}
	if core == "" {
		return trimmed
	}
	return core
}

// normalizeSimilarTitle 归一化标题：去空白与标点、转小写，用于去重与排除当前片。
func normalizeSimilarTitle(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, title)
}

// isSameTitleVariant 判断候选标题是不是当前片的同名变体：归一化后相等，
// 或一方包含另一方（第 X 季、4K 版、其它语言版、副标题都算同名变体）。
// 语言后缀（如阿拉伯语版）归一化后仍含核心片名，同样被排除。
func isSameTitleVariant(candidate, core string) bool {
	c := normalizeSimilarTitle(candidate)
	k := normalizeSimilarTitle(core)
	if c == "" || k == "" {
		return false
	}
	if c == k {
		return true
	}
	return strings.Contains(c, k) || strings.Contains(k, c)
}

// filterSimilarItems 从全网搜索结果里挑同类：去掉当前这部与一切同名变体、
// 按归一化标题去重（同名不同站只留一个）、截断到上限。
func filterSimilarItems(items []VodItem, currentTitle, excludeID string, cap int) []VodItem {
	if cap <= 0 {
		return nil
	}
	current := normalizeSimilarTitle(currentTitle)
	core := similarCoreTitle(currentTitle)
	seen := make(map[string]bool)
	var out []VodItem
	for _, it := range items {
		if excludeID != "" && it.ID == excludeID {
			continue
		}
		n := normalizeSimilarTitle(it.Title)
		if n == "" || n == current {
			continue
		}
		if isSameTitleVariant(it.Title, core) {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, it)
		if len(out) >= cap {
			break
		}
	}
	return out
}

// similarCategoryKeyword 按类型/地区归到一个大类词，用于在本站分类里
// 找同类栏目：电影 / 电视剧 / 综艺 / 动漫 / 纪录片；归不到返回空串。
func similarCategoryKeyword(vodType, area string) string {
	t := vodType + " " + area
	switch {
	case strings.Contains(t, "电影"):
		return "电影"
	case strings.Contains(t, "综艺"):
		return "综艺"
	case strings.Contains(t, "动漫"), strings.Contains(t, "动画"):
		return "动漫"
	case strings.Contains(t, "纪录"):
		return "纪录"
	case strings.Contains(t, "剧"):
		return "电视"
	default:
		return ""
	}
}

// similarSectionFor 在本站分类里找与大类词匹配的栏目：标题含大类词
// （电视类再认含「剧」的栏目，如「连续剧」「内地剧」）。
func similarSectionFor(sections []Section, keyword string) (Section, bool) {
	for _, sc := range sections {
		if keyword == "" {
			continue
		}
		if strings.Contains(sc.Title, keyword) {
			return sc, true
		}
		if keyword == "电视" && strings.Contains(sc.Title, "剧") {
			return sc, true
		}
		if keyword == "动漫" && strings.Contains(sc.Title, "动画") {
			return sc, true
		}
	}
	return Section{}, false
}

// browseSimilarCategory 浏览本站同类栏目第 1–2 页，取不同片名的条目。
// 找不到栏目、栏目打不开、或站点不支持浏览时返回空，不报错；
// 单个站点实现可能 panic（如测试里的桩），内部兜住只跳过。
func (s *ShellService) browseSimilarCategory(site, keyword, title, excludeID string) (out []VodItem) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("找同类浏览栏目内部错误，已跳过: %v", r)
			out = nil
		}
	}()
	if site == "" || keyword == "" {
		return nil
	}
	pv, err := s.vodOf(site)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), similarBrowseTimeout)
	defer cancel()
	sections, err := pv.Home(ctx)
	if err != nil {
		return nil
	}
	secs := make([]Section, len(sections))
	for i, sc := range sections {
		secs[i] = Section{ID: sc.ID, Title: sc.Title}
	}
	section, ok := similarSectionFor(secs, keyword)
	if !ok {
		return nil
	}
	for page := 1; page <= 2 && len(out) < similarMaxResults; page++ {
		pageCtx, cancelPage := context.WithTimeout(ctx, similarBrowseTimeout)
		pg, err := pv.Browse(pageCtx, section.ID, page)
		cancelPage()
		if err != nil {
			return out
		}
		items := toVodItems(pg.Items)
		for j := range items {
			items[j].Site = site
		}
		out = filterSimilarItems(append(out, items...), title, excludeID, similarMaxResults)
		if !pg.HasMore {
			break
		}
	}
	return out
}

// VodSimilar 找同类：先浏览本站同类栏目（不同片名），不够再按核心片名
// 全网搜补齐（同名变体不要）。搜不到返回空，不报错；单个站点失败/超时
// 只跳过该站。site 是当前详情的站点 key，vodType/vodArea 是详情的类型
// 与地区（用于找同类栏目）；传空时只走全网搜那路。
func (s *ShellService) VodSimilar(title, excludeID, site, vodType, vodArea string) ([]VodItem, error) {
	core := similarCoreTitle(title)
	if core == "" {
		return nil, nil
	}
	// 主路：本站同类栏目浏览。
	out := s.browseSimilarCategory(site, similarCategoryKeyword(vodType, vodArea), title, excludeID)
	if len(out) >= similarMaxResults {
		return out, nil
	}

	// 补路：核心片名全网搜。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.mu.RLock()
	sites := make([]string, 0, len(s.vods))
	providers := make([]provider.Provider, 0, len(s.vods))
	for k, pv := range s.vods {
		sites = append(sites, k)
		providers = append(providers, pv)
	}
	s.mu.RUnlock()

	threads := s.SearchThreads()
	if threads <= 0 {
		threads = 1
	}
	sem := make(chan struct{}, threads)
	var mu sync.Mutex
	var searched []VodItem
	var wg sync.WaitGroup
	for i, pv := range providers {
		wg.Add(1)
		go func(site string, pv provider.Provider) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("全网找同类站点 %q 内部错误，已跳过: %v", site, r)
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			if s.searchSiteSkipped(site, time.Now()) {
				return
			}
			siteCtx, cancelSite := context.WithTimeout(ctx, siteSearchTimeout)
			defer cancelSite()
			items, err := pv.Search(siteCtx, core)
			if err != nil {
				return
			}
			v := toVodItems(items)
			for j := range v {
				v[j].Site = site
			}
			mu.Lock()
			searched = append(searched, v...)
			mu.Unlock()
		}(sites[i], pv)
	}
	wg.Wait()
	// 合并：已有的不同片名在前，全网搜的补在后面，去重后截断。
	merged := append(append([]VodItem(nil), out...), searched...)
	return filterSimilarItems(merged, title, excludeID, similarMaxResults), nil
}
