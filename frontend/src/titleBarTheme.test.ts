import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import { titleBarColourForTheme } from './titleBarTheme'

const app = readFileSync('src/App.vue', 'utf8')

describe('标题栏跟背景色契约', () => {
  // 2026-11-19：窗口最上面那条白色标题栏太突兀，改成和用户选的背景一个色系。
  // 主题 → 标题栏纯色（渐变主题取首色）；未知主题回默认深色。
  it('每个主题都有对应的标题栏纯色', () => {
    expect(titleBarColourForTheme('default')).toBe('#141414')
    expect(titleBarColourForTheme('aurora')).toBe('#120324')
    expect(titleBarColourForTheme('wood')).toBe('#E2D5C0')
    expect(titleBarColourForTheme('ocean')).toBe('#4a8aaa')
    expect(titleBarColourForTheme('8bit')).toBe('#0e0e14')
    expect(titleBarColourForTheme('不存在的主题')).toBe('#141414')
  })

  it('换主题和启动加载主题时都同步标题栏颜色', () => {
    // syncTitleBarColour 是唯一调 SetTitleBarColour 的地方；换主题和启动加载主题都要调它。
    expect(app).toMatch(/async function syncTitleBarColour[\s\S]*?ShellService\.SetTitleBarColour\(/)
    const calls = app.match(/void syncTitleBarColour\(\)/g) ?? []
    expect(calls.length).toBe(2)
  })
})
