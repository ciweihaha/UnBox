// ephemeralMessage.ts — 短暂提示的自动消失。
//
// 底部错误条、成功提示这类消息只在当时有用：显示 5 秒后自动清空，
// 不用用户手动点 X。带操作按钮的消息（如证书一键豁免）不自动清，
// 用户还没点按钮就消失等于把修复入口藏起来了。
import { watch, type Ref } from 'vue'

/** 默认自动消失时长（毫秒）。 */
export const EPHEMERAL_MESSAGE_TIMEOUT_MS = 5000

/**
 * watch 一个消息 ref：非空时起定时器自动清空；新消息进来重新计时；
 * skip 为真时不自动清。返回一个停止函数（卸载时调用，清掉计时器与 watch）。
 */
export function autoClearMessage(
  message: Ref<string>,
  timeoutMs: number = EPHEMERAL_MESSAGE_TIMEOUT_MS,
  skip?: () => boolean,
): () => void {
  let timer: ReturnType<typeof setTimeout> | null = null
  const stop = watch(message, (value) => {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
    if (!value) return
    if (skip?.()) return
    timer = setTimeout(() => {
      message.value = ''
    }, timeoutMs)
  })
  return () => {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
    stop()
  }
}
