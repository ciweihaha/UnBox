package shell

// 「更多类似内容」改全网搜：用当前片名去所有点播站搜同类，不走 VodSearchAll
//（那条路会取消正在进行的用户搜索、还推搜索进度事件；详情页的类似内容是
// 后台静默查询）。不要 Key：只拿标题/季/集等现成信息去各站搜同类词汇总；
// 搜不到就返回空，前端显示空状态，不硬推不相关的。

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

// filterSimilarItems 从全网搜索结果里挑同类：去掉当前这部、按归一化标题
// 去重（同名不同站只留一个）、截断到上限。
func filterSimilarItems(items []VodItem, currentTitle, excludeID string, cap int) []VodItem {
	if cap <= 0 {
		return nil
	}
	current := normalizeSimilarTitle(currentTitle)
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

// VodSimilar 用当前片名去所有点播站搜同类（核心词查询），返回去重后的同类
// 片单。搜不到返回空，不报错；单个站点失败/超时只跳过该站。
func (s *ShellService) VodSimilar(title, excludeID string) ([]VodItem, error) {
	core := similarCoreTitle(title)
	if core == "" {
		return nil, nil
	}
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
	var out []VodItem
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
			out = append(out, v...)
			mu.Unlock()
		}(sites[i], pv)
	}
	wg.Wait()
	return filterSimilarItems(out, title, excludeID, similarMaxResults), nil
}
