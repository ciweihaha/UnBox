import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')
const cardList = readFileSync('src/components/ContentCardList.vue', 'utf8')

// 搜索结果「点播搜索」区（「点播搜索」注释到「点播收藏」注释之间）。
const searchSection = app.slice(
  app.indexOf('<!-- 点播搜索 -->'),
  app.indexOf('<!-- 点播收藏 -->'),
)

// 搜索结果页重做（第 3 项）：搜「图书馆员」得 8 条结果却只有纯文字行，
// 没有一张海报。根因：搜索区跟全局列表/卡片开关（contentCardStyle，
// 用户当前是列表模式）走；列表模式只渲染文字行，有 Logo 才给 44×60 小图，
// 且图坏了就静默隐藏、没有任何封面回退。修法：搜索结果一律按海报卡片网格
// 渲染（不受开关影响），无 Logo 用现有 posterGradient 渐变封面 + 首字符。
describe('vod search redesign', () => {
  it('always renders search results as a poster grid, ignoring the list/card switch', () => {
    expect(searchSection).toContain(":style=\"'grid'\"")
    expect(searchSection).not.toContain(':style="contentCardStyle"')
  })

  it('shows a gradient cover with the first character when a result has no poster', () => {
    expect(cardList).toContain('content-card-fallback')
    expect(cardList).toContain('posterGradient(item.title)')
  })

  it('clamps search result titles to at most 2 lines', () => {
    expect(stylesheet).toMatch(/\.search-results \.content-card-title\s*\{[^}]*-webkit-line-clamp:\s*2;/s)
  })

  it('shows a styled loading state in the results area while searching', () => {
    expect(app).toContain('v-if="searching && !vodSearchItems.length"')
    expect(app).toContain('class="search-loading"')
    expect(stylesheet).toMatch(/\.search-loading\s*\{[^}]*text-align:\s*center;/s)
  })

  it('keeps the empty state styled and the count and the strong/weak divider', () => {
    expect(searchSection).toContain('暂无搜索结果')
    expect(stylesheet).toMatch(/\.search-page \.home-empty\s*\{[^}]*text-align:\s*center;/s)
    expect(searchSection).toContain('找到 {{ vodSearchItems.length }} 条结果')
    expect(searchSection).toContain(':divider-after="searchStrongCount"')
    expect(searchSection).toContain('divider-label="弱相关结果"')
  })
})
