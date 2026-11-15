<script setup lang="ts">
// VodFilterPanel.vue — 点播筛选 chip 行：按维度渲染单选 chips，点选即回传。
// 与旧版下拉同一份数据模型（FilterGroup[] + modelValue，nextSelections 单选
// 替换），只改呈现：下拉要两次点击（展开→选），chip 一次点选直接生效；
// 未传选中值时默认第一项（一般即「全部」）高亮，与 initialSelections 一致。
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/provider/models'
import { nextSelections } from '../vodFilters'

const props = defineProps<{
  groups: FilterGroup[]
  modelValue: Record<string, string>
}>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

function onPick(key: string, value: string) {
  emit('update:modelValue', nextSelections(props.modelValue, key, value))
}

function activeValue(key: string): string {
  if (props.modelValue[key] !== undefined) return props.modelValue[key]
  return (props.groups.find(g => g.key === key)?.options ?? [])[0]?.value ?? ''
}
</script>

<template>
  <div class="vod-filter-panel">
    <div v-for="g in groups" :key="g.key" class="vod-filter-dim">
      <span class="vod-filter-name">{{ g.name }}</span>
      <span class="vod-filter-chips">
        <button
          v-for="o in g.options"
          :key="o.value"
          type="button"
          class="vod-filter-chip"
          :class="{ active: activeValue(g.key) === o.value }"
          :aria-pressed="activeValue(g.key) === o.value"
          @click="onPick(g.key, o.value)"
        >{{ o.name }}</button>
      </span>
    </div>
  </div>
</template>
