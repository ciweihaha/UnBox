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
  Provider: 'free',
  FreeProvider: 'azure',
  FreeAPIKey: 'azure-key-1',
  AzureRegion: 'eastasia',
  CFAccountID: '',
  CFToken: '',
}

describe('createTranslateSettings', () => {
  it('缺失或非法值归一化为默认（开关关、免费接口、Azure）', async () => {
    const store = createTranslateSettings(api(async () => ({}) as never))
    await store.load()
    expect(store.settings.value).toEqual({
      Enabled: false,
      Provider: 'off',
      FreeProvider: 'azure',
      FreeAPIKey: '',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
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

    await store.setProvider('off')
    expect(store.settings.value.Enabled).toBe(false)
    expect(set).toHaveBeenCalledWith({ ...stored, Provider: 'off', Enabled: false })
  })

  it('改服务商与 Key 时去首尾空白；非法服务商回退 Azure', async () => {
    const set = vi.fn(async () => {})
    const store = createTranslateSettings(api(async () => ({ ...stored }), set))
    await store.load()

    await store.setFreeProvider('cloudflare')
    expect(set).toHaveBeenCalledWith({ ...stored, FreeProvider: 'cloudflare' })
    await store.setFreeAPIKey('  azure-key-2  ')
    expect(store.settings.value.FreeAPIKey).toBe('azure-key-2')
    expect(set).toHaveBeenCalledWith({ ...stored, FreeProvider: 'cloudflare', FreeAPIKey: 'azure-key-2' })
    await store.setFreeProvider('ollama' as never)
    expect(store.settings.value.FreeProvider).toBe('azure')
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

    await store.setProvider('free')
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

  it('checkModel：后端就绪返回 true，接口没通返回 false 并上报错误', async () => {
    const ready = createTranslateSettings(api(async () => ({ ...stored }), async () => {}, async () => {}))
    await expect(ready.checkModel()).resolves.toBe(true)

    const failure = new Error('未填 Azure API Key')
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
  it('设置页有字幕翻译区：免费接口/关闭、服务商、Key、检测接口', () => {
    expect(app).toContain('字幕翻译')
    expect(app).toContain('createTranslateSettings')
    expect(app).toContain('translateSettingsStore.load()')
    expect(app).toContain('setTranslateProvider(')
    expect(app).toContain('translateFreeAPIKey')
    expect(app).toContain('checkTranslateModel')
    expect(app).toContain('免费接口')
    expect(app).toContain('Azure')
    expect(app).toContain('Cloudflare')
    expect(app).not.toContain('v-model="translateModel"')
    expect(app).not.toContain('v-model="translateEndpoint"')
    expect(app).not.toContain('toggleTranslateSetting(')
  })

  it('两个播放器都接翻译开关与翻译函数', () => {
    const hits = app.match(/:translate-enabled="translateSettings\.Enabled"/g) ?? []
    expect(hits.length).toBe(2)
    expect(app).toContain(':translate-text="translateSubtitleText"')
  })
})
