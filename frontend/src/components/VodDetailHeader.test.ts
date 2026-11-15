import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VodDetailHeader from './VodDetailHeader.vue'

// 详情弹窗重做（第 2 项）：「正在播放：…」不再飘红字在弹窗顶部，
// 改收进播放器上方做状态行；弹窗头只剩返回按钮。
describe('VodDetailHeader', () => {
  it('keeps only the back action in the header', () => {
    const wrapper = mount(VodDetailHeader)
    const row = wrapper.find('.vod-detail-head')
    expect(row.find('button.vod-back').exists()).toBe(true)
    expect(row.find('.now').exists()).toBe(false)
    expect(row.element.children).toHaveLength(1)
  })
})
