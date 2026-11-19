import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')

describe('直播两栏布局契约', () => {
  // 2026-11-19：直播页改成和 TiviMateWin 一样两栏——左边频道列表、右边播放区；
  // 左边那条分类栏和右边的频道列表分栏都不要了，两边都摆东西看着乱。
  it('直播页是两栏：左边频道列表、右边播放区', () => {
    expect(stylesheet).toMatch(/\.layout\.live\s*\{[^}]*grid-template-columns:\s*minmax\(280px, 380px\) minmax\(320px, 1fr\);/s)
    expect(app).not.toContain('class="groups"')
  })

  it('左边栏顶部有搜索框 + 分类下拉框 + 收藏星标筛选', () => {
    expect(app).toMatch(/liveGroupOptions/)
    expect(app).toMatch(/ThemedSelect[\s\S]*?aria-label="分类"/)
    expect(app).toMatch(/toggleLiveFavorites/)
  })

  it('频道每行点行就播，只留一个星标收藏，不再带播放按钮', () => {
    expect(app).toMatch(/<li v-for="c in channels"[^>]*@click="play\(c\)"/)
    expect(app).not.toMatch(/<button title="播放"[^>]*@click="play\(c\)"/)
    expect(app).toMatch(/@click\.stop="toggleFav\(c\)"/)
  })

  it('分类下的频道列表能内部滚动（宽屏）', () => {
    // 2026-11-19 用户纠正：分类下面频道列表滑不动。根因和点播列表同类——
    // 网格行没约束、.channels 缺 min-height: 0，ul 被内容撑满、overflow 永远不生效。
    // 宽屏要求：网格行 minmax(0, 1fr) 约束高度，.channels 和 ul 都能缩到剩余空间再滚动。
    expect(stylesheet).toMatch(/\.layout\.live\s*\{[^}]*grid-template-rows:\s*minmax\(0, 1fr\);/s)
    expect(stylesheet).toMatch(/\.layout\.live \.channels\s*\{[^}]*min-height:\s*0;/s)
    expect(stylesheet).toMatch(/\.channels ul\s*\{[^}]*flex:\s*1;[^}]*min-height:\s*0;[^}]*overflow-y:\s*auto;/s)
  })
})
