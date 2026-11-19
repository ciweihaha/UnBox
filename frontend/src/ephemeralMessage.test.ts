// ephemeralMessage.test.ts — 底部错误条/成功提示的自动消失。
//
// 用户反馈：点“检测接口”失败后，底部红条一直挂着，非得手动点 X。
// 约定：消息非空后 5 秒自动清空；带操作按钮的消息（如证书一键豁免）
// 不自动清，等用户自己处理；新消息进来重新计时。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { autoClearMessage } from './ephemeralMessage'

describe('autoClearMessage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('消息出现 5 秒后自动清空', async () => {
    const msg = ref('')
    autoClearMessage(msg)
    msg.value = '免费翻译接口返回 HTTP 401'
    await nextTick()
    vi.advanceTimersByTime(4999)
    expect(msg.value).toBe('免费翻译接口返回 HTTP 401')
    vi.advanceTimersByTime(1)
    expect(msg.value).toBe('')
  })

  it('新消息进来重新计时，旧的计时不提前清掉新消息', async () => {
    const msg = ref('')
    autoClearMessage(msg)
    msg.value = '第一条错误'
    await nextTick()
    vi.advanceTimersByTime(4000)
    msg.value = '第二条错误'
    await nextTick()
    vi.advanceTimersByTime(4000)
    expect(msg.value).toBe('第二条错误')
    vi.advanceTimersByTime(1000)
    expect(msg.value).toBe('')
  })

  it('手动清空后计时器不再复活消息', async () => {
    const msg = ref('')
    autoClearMessage(msg)
    msg.value = '一条错误'
    await nextTick()
    msg.value = ''
    await nextTick()
    vi.advanceTimersByTime(6000)
    expect(msg.value).toBe('')
  })

  it('skip 为真时不自动清（比如带一键豁免按钮的证书错误）', async () => {
    const msg = ref('')
    const skip = ref(true)
    autoClearMessage(msg, 5000, () => skip.value)
    msg.value = '证书过期'
    await nextTick()
    vi.advanceTimersByTime(10000)
    expect(msg.value).toBe('证书过期')
    skip.value = false
    msg.value = ''
    await nextTick()
    msg.value = '证书过期'
    await nextTick()
    vi.advanceTimersByTime(5000)
    expect(msg.value).toBe('')
  })

  it('成功提示也一样自动消失', async () => {
    const msg = ref('')
    autoClearMessage(msg)
    msg.value = '免费接口就绪，可以开字幕翻译'
    await nextTick()
    vi.advanceTimersByTime(5000)
    expect(msg.value).toBe('')
  })
})
