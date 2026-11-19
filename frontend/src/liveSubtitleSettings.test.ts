// liveSubtitleSettings.test.ts — 直播实时字幕设置的加载、持久化与归一化。
import { describe, expect, it, vi } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import {
  createLiveSubtitleSettings,
  liveSubtitleStatusText,
  normalizeLiveSubtitleSettings,
  type LiveSubtitleSettings,
  type LiveSubtitleSettingsApi,
  type LiveSubtitleStatus,
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

describe('liveSubtitleStatusText', () => {
  const base: LiveSubtitleStatus = {
    Running: false,
    Enabled: true,
    WhisperModel: 'tiny.en',
    WindowSeconds: 1.5,
    WindowsProcessed: 0,
    WindowsDropped: 0,
    LastLatencyMs: 0,
    AvgLatencyMs: 0,
    LastSubtitle: '',
    Error: '',
  }

  it('运行中报延迟', () => {
    expect(liveSubtitleStatusText({ ...base, Running: true, LastLatencyMs: 2300 })).toBe(
      '直播字幕运行中，最近延迟约 2.3 秒',
    )
  })

  it('有错误报异常原因', () => {
    expect(liveSubtitleStatusText({ ...base, Error: '模型下载失败' })).toBe('直播字幕异常：模型下载失败')
  })

  it('开关关着时说清楚是已关闭、不是故障', () => {
    const text = liveSubtitleStatusText({ ...base, Enabled: false })
    expect(text).toContain('已关闭')
    expect(text).not.toContain('未启动')
  })

  it('开着但没在播时说清楚是待机、什么时候才启动', () => {
    // 引擎只在播英文直播（网页播放）时才启动：设置页里没在播，
    // “开着却未启动”是正常待机，必须写明原因，不能只说“未启动”。
    const text = liveSubtitleStatusText(base)
    expect(text).toContain('尚未启动')
    expect(text).toContain('播英文直播')
    expect(text).not.toBe('直播字幕未启动')
  })

  it('状态查不到时明说查不到', () => {
    expect(liveSubtitleStatusText(null)).toContain('查不到')
  })
})
