// liveSubtitleSettings.test.ts — 直播实时字幕设置的加载、持久化与归一化。
import { describe, expect, it, vi } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import {
  createLiveSubtitleSettings,
  normalizeLiveSubtitleSettings,
  type LiveSubtitleSettings,
  type LiveSubtitleSettingsApi,
} from './liveSubtitleSettings'

describe('启动加载', () => {
  it('应用启动时加载直播字幕设置，不只在进设置页时加载', () => {
    // 2026-11-18 回归：启动时只加载了播放设置，直播字幕设置默认是关的，
    // 导致首次播放后端管线已起、前端却不抓音频，一直没有字幕；
    // 只有进过设置页才加载，开关才变成开。
    const app = readFileSync('src/App.vue', 'utf8')
    const mounted = app.slice(app.indexOf('onMounted(() => {'), app.indexOf('onBeforeUnmount('))
    expect(mounted).toContain('liveSubtitleSettingsStore.load()')
  })
})

function stubApi(initial: Partial<LiveSubtitleSettings> | null = null) {
  let stored: LiveSubtitleSettings = normalizeLiveSubtitleSettings(initial)
  const api: LiveSubtitleSettingsApi = {
    GetLiveSubtitleSettings: vi.fn(async () => ({ ...stored })),
    SetLiveSubtitleSettings: vi.fn(async (s: LiveSubtitleSettings) => {
      stored = { ...s }
    }),
    LiveSubtitleStatus: vi.fn(async () => null),
  }
  return { api, getStored: () => stored }
}

describe('normalizeLiveSubtitleSettings', () => {
  it('空值回退默认', () => {
    expect(normalizeLiveSubtitleSettings(null)).toEqual({
      Enabled: false,
      WhisperModel: 'tiny.en',
      WindowSeconds: 1.5,
    })
  })

  it('非法窗口回退默认', () => {
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: 0 }).WindowSeconds).toBe(1.5)
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: -1 }).WindowSeconds).toBe(1.5)
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: 99 }).WindowSeconds).toBe(1.5)
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: NaN }).WindowSeconds).toBe(1.5)
  })

  it('合法值保留', () => {
    expect(
      normalizeLiveSubtitleSettings({ Enabled: true, WhisperModel: 'tiny.en', WindowSeconds: 1.5 }),
    ).toEqual({ Enabled: true, WhisperModel: 'tiny.en', WindowSeconds: 1.5 })
  })

  it('空模型名回退默认', () => {
    expect(normalizeLiveSubtitleSettings({ WhisperModel: '   ' }).WhisperModel).toBe('tiny.en')
  })
})

describe('createLiveSubtitleSettings', () => {
  it('load 从后端读设置', async () => {
    const { api } = stubApi({ Enabled: true, WhisperModel: 'tiny.en', WindowSeconds: 1.5 })
    const store = createLiveSubtitleSettings(api)
    await store.load()
    expect(store.settings.value).toEqual({ Enabled: true, WhisperModel: 'tiny.en', WindowSeconds: 1.5 })
  })

  it('setEnabled 先更新界面再持久化', async () => {
    const { api, getStored } = stubApi()
    const store = createLiveSubtitleSettings(api)
    await store.setEnabled(true)
    expect(store.settings.value.Enabled).toBe(true)
    expect(getStored().Enabled).toBe(true)
  })

  it('setWhisperModel 与 setWindowSeconds 归一化后持久化', async () => {
    const { api, getStored } = stubApi()
    const store = createLiveSubtitleSettings(api)
    await store.setWhisperModel('tiny.en')
    expect(getStored().WhisperModel).toBe('tiny.en')
    await store.setWindowSeconds(0)
    expect(store.settings.value.WindowSeconds).toBe(1.5)
    expect(getStored().WindowSeconds).toBe(1.5)
  })

  it('status 失败返回 null、不抛错', async () => {
    const { api } = stubApi()
    api.LiveSubtitleStatus = vi.fn(async () => {
      throw new Error('nope')
    })
    const onError = vi.fn()
    const store = createLiveSubtitleSettings(api, onError)
    expect(await store.status()).toBeNull()
    expect(onError).toHaveBeenCalled()
  })
})
