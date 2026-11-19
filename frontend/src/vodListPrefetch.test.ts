import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const app = readFileSync('src/App.vue', 'utf8')

describe('点播分类多拉几页契约', () => {
  // 2026-11-19：首页每个分类下面资源太少——换分类后默认多拉两页（共 3 页），
  // 从全网（44 站合一的聚合源）扩充；后台静默追加，不挡首屏；切分类立刻停。
  it('换分类（重置加载）成功后后台追加预取页', () => {
    expect(app).toContain('prefetchVodPages')
    expect(app).toMatch(/VOD_LIST_PREFETCH_PAGES/)
    expect(app).toMatch(/if \(reset && .*?\) void prefetchVodPages\(\)/s)
  })

  it('预取时检查还有更多页、分类没换、不在加载中', () => {
    expect(app).toMatch(/prefetchVodPages[\s\S]*?vodHasMore\.value/s)
    expect(app).toMatch(/prefetchVodPages[\s\S]*?vodActiveCat\.value !== cat/s)
    expect(app).toMatch(/prefetchVodPages[\s\S]*?vodListLoading\.value/s)
  })
})
