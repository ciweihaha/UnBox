import { describe, expect, it, vi } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import {
  createTranslateSettings,
  type TranslateSettings,
  type TranslateSettingsApi,
} from './translateSettings'

const app = readFileSync('src/App.vue', 'utf8')

function api(
  get: TranslateSettingsApi['GetTranslateSettings'],
  set: TranslateSettingsApi['SetTranslateSettings'] = async () => {},
  check: TranslateSettingsApi['CheckTranslateModel'] = async () => {},
): TranslateSettingsApi {
  return { GetTranslateSettings: get, SetTranslateSettings: set, CheckTranslateModel: check }
}

const stored: TranslateSettings = {
  Enabled: true,
  Model: 'qwen2.5:7b',
  Endpoint: 'http://127.0.0.1:11434',
}

describe('createTranslateSettings', () => {
  it('缺失或非法值归一化为默认（开关关、3B、本地地址）', async () => {
    const store = createTranslateSettings(api(async () => ({}) as never))
    await store.load()
    expect(store.settings.value).toEqual({
      Enabled: false,
      Model: 'qwen2.5:3b',
      Endpoint: 'http://127.0.0.1:11434',
    })
  })

  it('读取已有设置并原样回显', async () => {
    const store = createTranslateSettings(api(async () => ({ ...stored })))
    await store.load()
    expect(store.settings.value).toEqual(stored)
  })

  it('切换开关立即更新运行时值并提交完整聚合对象', async () => {
    const set = vi.fn(async () => {})
    const store = createTranslateSettings(api(async () => ({ ...stored }), set))
    await store.load()

    await store.setEnabled(false)
    expect(store.settings.value.Enabled).toBe(false)
    expect(set).toHaveBeenCalledWith({ ...stored, Enabled: false })
  })

  it('改模型与地址时去首尾空白；留空回退默认并提交默认值', async () => {
    const set = vi.fn(async () => {})
    const store = createTranslateSettings(api(async () => ({ ...stored }), set))
    await store.load()

    await store.setModel('  qwen2.5:7b  ')
    expect(set).toHaveBeenCalledWith({ ...stored, Model: 'qwen2.5:7b' })
    await store.setEndpoint('   ')
    expect(store.settings.value.Endpoint).toBe('http://127.0.0.1:11434')
    expect(set).toHaveBeenCalledWith({ ...stored, Model: 'qwen2.5:7b', Endpoint: 'http://127.0.0.1:11434' })
  })

  it('持久化失败保留运行时值并上报错误', async () => {
    const failure = new Error('store closed')
    const onError = vi.fn()
    const store = createTranslateSettings(
      api(async () => ({}) as never, async () => {
        throw failure
      }),
      onError,
    )
    await store.load()

    await store.setEnabled(true)
    expect(store.settings.value.Enabled).toBe(true)
    expect(onError).toHaveBeenCalledWith(failure)
  })

  it('读取失败保持默认并上报错误', async () => {
    const failure = new Error('db unavailable')
    const onError = vi.fn()
    const store = createTranslateSettings(
      api(async () => {
        throw failure
      }),
      onError,
    )
    await store.load()
    expect(store.settings.value.Enabled).toBe(false)
    expect(onError).toHaveBeenCalledWith(failure)
  })

  it('checkModel：后端就绪返回 true，模型未装/服务没起返回 false 并上报错误', async () => {
    const ready = createTranslateSettings(api(async () => ({ ...stored }), async () => {}, async () => {}))
    await expect(ready.checkModel()).resolves.toBe(true)

    const failure = new Error('本地未找到模型 qwen2.5:7b')
    const onError = vi.fn()
    const missing = createTranslateSettings(
      api(async () => ({ ...stored }), async () => {}, async () => {
        throw failure
      }),
      onError,
    )
    await expect(missing.checkModel()).resolves.toBe(false)
    expect(onError).toHaveBeenCalledWith(failure)
  })
})

describe('设置页字幕翻译接线', () => {
  it('设置页有字幕翻译区：开关、模型、地址、检测模型', () => {
    expect(app).toContain('字幕翻译')
    expect(app).toContain('createTranslateSettings')
    expect(app).toContain('translateSettingsStore.load()')
    expect(app).toContain('toggleTranslateSetting(')
    expect(app).toContain('translateModel')
    expect(app).toContain('translateEndpoint')
    expect(app).toContain('checkTranslateModel')
  })

  it('三个播放器都接翻译开关与翻译函数', () => {
    const hits = app.match(/:translate-enabled="translateSettings\.Enabled"/g) ?? []
    expect(hits.length).toBe(3)
    expect(app).toContain(':translate-text="translateSubtitleText"')
  })
})
