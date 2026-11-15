import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')
const cardList = readFileSync('src/components/ContentCardList.vue', 'utf8')

// 首页「为你精选」行（模板里的 <h2>为你精选 到下一行「我的片单」之间）。
const featuredStart = app.indexOf('<h2>为你精选')
const featuredSection = app.slice(
  featuredStart,
  app.indexOf('我的片单', featuredStart),
)

// 首页「为你精选」改海报卡片（第 4 项）：用户截图里 9 个片名只有文字，
// 没有一张海报。根因不是数据（条目经 vodCardItem 带 Logo）也不是模板
// （这一行一直走 ContentCardList 卡片网格），而是样式：海报框
// .content-card-poster 是卡片纵向 flex 容器里的子项，它里面全是绝对定位
// 的子元素（img / 渐变封面），内容高度为 0；aspect-ratio 2:3 就按 0 高度
// 反推宽度，海报框塌成 2×3px（只剩边框），肉眼就是一排纯文字。
// 本地 harness 实测：未修时 10 张卡全是纯文字行；修后海报/渐变封面全现。
// 修法：给海报框一个确定宽度，aspect-ratio 再按宽度推高度。
describe('vod featured row redesign', () => {
  it('gives the poster box a definite width so aspect-ratio can size it', () => {
    expect(stylesheet).toMatch(/\.content-card-poster\s*\{[^}]*width:\s*100%;/s)
  })

  it('renders the featured row as poster cards through ContentCardList grid', () => {
    expect(featuredSection).toContain('ContentCardList')
    expect(featuredSection).toContain('list-class="netflix-row"')
    expect(featuredSection).toContain(":style=\"'grid'\"")
  })

  it('feeds the featured row from the logo-carrying card items', () => {
    expect(app).toMatch(/featuredRowItems\s*=\s*computed\(\(\) => homeListCardItems\.value\.slice\(0,\s*10\)\)/)
    expect(app).toMatch(/homeListCardItems\s*=\s*computed<ContentCardItem\[\]>\(\(\) => vodListItems\.value\.map\(vodCardItem\)\)/)
    expect(cardList).toContain('content-card-fallback')
  })
})
