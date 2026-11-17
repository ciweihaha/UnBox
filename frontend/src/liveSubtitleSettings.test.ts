// liveSubtitleSettings.test.ts — 直播实时字幕设置的加载、持久化与归一化。
import { describe, expect, it, vi } from 'vitest'
import {
  createLiveSubtitleSettings,
  normalizeLiveSubtitleSettings,
  type LiveSubtitleSettings,
  type LiveSubtitleSettingsApi,
} from './liveSubtitleSettings'

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
      WhisperModel: 'base.en',
      WindowSeconds: 2,
    })
  })

  it('非法窗口回退默认', () => {
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: 0 }).WindowSeconds).toBe(2)
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: -1 }).WindowSeconds).toBe(2)
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: 99 }).WindowSeconds).toBe(2)
    expect(normalizeLiveSubtitleSettings({ WindowSeconds: NaN }).WindowSeconds).toBe(2)
  })

  it('合法值保留', () => {
    expect(
      normalizeLiveSubtitleSettings({ Enabled: true, WhisperModel: 'tiny.en', WindowSeconds: 1.5 }),
    ).toEqual({ Enabled: true, WhisperModel: 'tiny.en', WindowSeconds: 1.5 })
  })

  it('空模型名回退默认', () => {
    expect(normalizeLiveSubtitleSettings({ WhisperModel: '   ' }).WhisperModel).toBe('base.en')
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
    expect(store.settings.value.WindowSeconds).toBe(2)
    expect(getStored().WindowSeconds).toBe(2)
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
