import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const app = readFileSync('src/App.vue', 'utf8')

describe('更多类似内容全网搜契约', () => {
  // 2026-11-19：类似内容不再只推当前源里的 8 条，改成拿当前片名去全网搜同类；
  // 不要新 Key；搜不到显示空状态，不硬推不相关的。
  it('打开详情后调全网找同类接口，而不是用当前列表拼类似内容', () => {
    expect(app).toMatch(/ShellService\.VodSimilar\(/)
    expect(app).not.toContain('similarSourceItems')
  })

  it('找同类时显示加载中，搜不到显示空状态', () => {
    expect(app).toContain('正在全网找同类')
    expect(app).toContain('全网没找到同类内容')
  })
})
