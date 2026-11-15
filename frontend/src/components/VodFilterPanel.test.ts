import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VodFilterPanel from './VodFilterPanel.vue'
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/provider/models'

const groups: FilterGroup[] = [
  { key: 'area', name: '地区', options: [{ name: '全部', value: '' }, { name: '内地', value: '内地' }] },
]

describe('VodFilterPanel', () => {
  it('按维度渲染 chips，点选一次即回传', async () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '' } } })
    expect(wrapper.text()).toContain('地区')
    expect(wrapper.findAll('select')).toHaveLength(0)
    const chips = wrapper.findAll('.vod-filter-chip')
    expect(chips.map(c => c.text())).toEqual(['全部', '内地'])
    expect(chips[0].classes()).toContain('active')

    await chips[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([{ area: '内地' }])
  })

  it('预选值回显为当前选中', () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '内地' } } })
    const chips = wrapper.findAll('.vod-filter-chip')
    expect(chips[1].classes()).toContain('active')
    expect(chips[0].classes()).not.toContain('active')
  })

  it('未传选中值时默认第一项高亮', () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: {} } })
    const chips = wrapper.findAll('.vod-filter-chip')
    expect(chips[0].classes()).toContain('active')
  })
})
