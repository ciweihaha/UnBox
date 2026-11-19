import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const app = readFileSync('src/App.vue', 'utf8')

// 首页分类栏（<div class="home-cats"> 所在行起，到下一行 <div class="home-rows"> 之间）。
const homeCatsDiv = app.indexOf('class="home-cats"')
const homeCatsSection = app.slice(app.lastIndexOf('<div', homeCatsDiv), app.indexOf('class="home-rows"', homeCatsDiv))

// 首页分类栏只留电影 / 电视剧 / 动漫 / 综艺四项（第 6 项）：
// 分类是源站下发的（vodCategories），各源命名不一（电影片、连续剧、剧集、
// 动画……），所以不能按相等过滤，要按名包含匹配；每类只取第一个命中的，
// 同一类出两个芯片（如电视剧 + 连续剧）就又不止四项了。
// 点播页的类目栏已删除（v0.9.14），分类只在顶部导航（电影/电视/综艺/儿童）。
describe('vod home category bar', () => {
  it('renders the home bar from the four main categories, not the raw source list', () => {
    expect(homeCatsSection).toContain('v-if="homeMainCats.length"')
    expect(homeCatsSection).toContain('v-for="c in homeMainCats"')
    expect(homeCatsSection).not.toContain('v-for="c in vodCategories"')
  })

  it('matches the four names with per-source variant tolerance', () => {
    expect(app).toMatch(/homeMainCats\s*=\s*computed/)
    // 电影（含电影片）、电视剧（含连续剧、剧集，与顶部导航 goVodCategoryByTitle 同一套别名）、
    // 动漫（含动画）、综艺（含综艺节目）。
    expect(app).toContain("'电影'")
    expect(app).toContain("'电视剧'")
    expect(app).toContain("'连续剧'")
    expect(app).toContain("'剧集'")
    expect(app).toContain("'动漫'")
    expect(app).toContain("'动画'")
    expect(app).toContain("'综艺'")
    // 按包含匹配，不按相等（电影片这类变体名也要命中）。
    expect(app).toMatch(/homeMainCats[\s\S]{0,600}\.includes\(n\)/)
  })

  it('takes only the first matching category per group, so at most four chips', () => {
    expect(app).toMatch(/homeMainCats[\s\S]{0,600}\.find\(/)
  })

  it('点播页不再有类目栏：分类只在顶部导航（电影/电视/综艺/儿童）', () => {
    expect(app).not.toContain('class="vod-cats"')
    expect(app).not.toContain('class="cats-list"')
    expect(app).toContain('TOP_NAV_CATEGORIES')
    expect(app).toContain('goTopNavCategory')
  })
})
