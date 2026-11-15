import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const app = readFileSync('src/App.vue', 'utf8')

// 点播列表页加载状态（第 1 项）：电影分类下内容区全空、无任何报错。
// 根因：VodListFiltered 调用一直不返回时，前端没有任何加载中/失败/空结果
// 状态——面板空白、底部错误条也不出现，用户看不到任何原因、也没有重试入口。
// 约定：加载中、失败、空结果三态都在内容区内明示，失败显示真实原因并给重试。
describe('vod list states', () => {
  it('tracks the list error in its own state', () => {
    expect(app).toContain("const vodListError = ref('')")
  })

  it('records the load failure reason into vodListError', () => {
    expect(app).toContain('vodListError.value = ')
  })

  it('shows a loading state in the list area while the first page loads', () => {
    expect(app).toMatch(/v-if="vodListLoading && !vodListItems\.length"/)
    expect(app).toContain('正在加载…')
  })

  it('shows the real failure reason with a retry entry when the list fails', () => {
    expect(app).toMatch(/v-else-if="vodListError && !vodListItems\.length"/)
    expect(app).toContain('{{ vodListError }}')
    expect(app).toContain('重新加载')
    expect(app).toContain('@click="reloadVodList()"')
  })

  it('shows an empty state with a retry entry when the list loads empty', () => {
    expect(app).toMatch(/v-else-if="!vodListItems\.length && vodListEnd"/)
    expect(app).toContain('该分类暂无内容')
  })
})
