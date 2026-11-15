<script setup lang="ts">
// 详情弹窗头只剩返回按钮；「正在播放：…」已收进播放器上方做状态行，不再飘红字在顶部。
defineEmits<{ back: [] }>()
</script>

<template>
  <div class="vod-detail-head">
    <button class="vod-back" type="button" @click="$emit('back')">← 返回</button>
  </div>
</template>
