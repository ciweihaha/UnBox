import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')

// 取样式表里某选择器的第一条规则体（行首开始算，复合选择器的尾部不算；
// 主题覆盖在后，第一条即默认主题的基础规则）。
function ruleBody(selector: string): string {
  const m = stylesheet.match(new RegExp('^' + selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*\\{[^}]*\\}', 'ms'))
  return m ? m[0] : ''
}

// 菜单纵深（第 8 项）：顶部导航、下拉菜单、卡片按层级给三档阴影，
// 导航 < 卡片 < 浮层菜单；菜单弹出带轻微位移 + 阴影加深，不再扁平一片。
// 现状：.topnav 全程无阴影；.history-dropdown / .rate-menu / .track-menu
// 都是浅 0 4px 16px；卡片静止态无阴影、只有 hover 有。
describe('menu depth', () => {
  it('defines three shadow tiers as variables in :root', () => {
    expect(stylesheet).toMatch(/--shadow-nav:\s*0 4px 16px/)
    expect(stylesheet).toMatch(/--shadow-card:\s*0 2px 8px/)
    expect(stylesheet).toMatch(/--shadow-card-hover:\s*0 8px 24px/)
    // 菜单最深：双层投影，模糊半径最大。
    expect(stylesheet).toMatch(/--shadow-menu:\s*0 12px 32px[^;]*, 0 2px 8px/)
  })

  it('gives the solid top nav a shadow', () => {
    expect(ruleBody('.topnav.solid')).toContain('box-shadow: var(--shadow-nav)')
    expect(ruleBody('.topnav')).toMatch(/transition:[^;]*box-shadow/)
  })

  it('gives cards a resting shadow and a deeper hover shadow', () => {
    expect(ruleBody('.content-card-poster')).toContain('box-shadow: var(--shadow-card)')
    expect(ruleBody('.content-card-grid .content-card:hover .content-card-poster')).toContain('box-shadow: var(--shadow-card-hover)')
  })

  it('gives floating menus the deepest shadow', () => {
    expect(ruleBody('.history-dropdown')).toContain('box-shadow: var(--shadow-menu)')
    expect(ruleBody('.rate-menu')).toContain('box-shadow: var(--shadow-menu)')
    expect(ruleBody('.track-menu')).toContain('box-shadow: var(--shadow-menu)')
  })

  it('pops menus open with a slight offset', () => {
    expect(stylesheet).toMatch(/@keyframes menu-pop\s*\{[^}]*translateY\(-4px\)/s)
    expect(stylesheet).toMatch(/@keyframes menu-pop-up\s*\{[^}]*translateY\(4px\)/s)
    expect(ruleBody('.history-dropdown')).toContain('animation: menu-pop')
    expect(ruleBody('.rate-menu')).toContain('animation: menu-pop-up')
    expect(ruleBody('.track-menu')).toContain('animation: menu-pop-up')
  })
})
