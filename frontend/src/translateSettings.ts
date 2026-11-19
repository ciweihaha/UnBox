// translateSettings.ts — 设置页「字幕翻译」分类的加载、持久化与接口检测。
//
// 翻译默认关；服务商缺失/非法时默认 Azure。切换时先更新运行时值再持久化，
// 持久化失败保留运行时值并把错误交给调用方展示（与 playbackSettings.ts 同一模式）。
import { ref, type Ref } from 'vue'
import {
  normalizeTranslateSettings,
  type FreeTranslateProvider,
  type TranslateProvider,
  type TranslateSettings,
} from './subtitleTranslate'

export type { TranslateSettings, TranslateProvider, FreeTranslateProvider }

/** 后端设置接口的最小形状（对应 ShellService 的三个绑定方法）。 */
export interface TranslateSettingsApi {
  GetTranslateSettings(): Promise<Partial<TranslateSettings> | null | undefined>
  SetTranslateSettings(settings: TranslateSettings): Promise<unknown>
  CheckTranslateModel(): Promise<unknown>
}

export interface TranslateSettingsStore {
  settings: Ref<TranslateSettings>
  load: () => Promise<void>
  setProvider: (value: TranslateProvider) => Promise<void>
  setFreeProvider: (value: FreeTranslateProvider) => Promise<void>
  setFreeAPIKey: (value: string) => Promise<void>
  setAzureRegion: (value: string) => Promise<void>
  setCFAccountID: (value: string) => Promise<void>
  setCFToken: (value: string) => Promise<void>
  /** checkModel 检测当前免费接口是否可用；就绪返回 true，否则 false。 */
  checkModel: () => Promise<boolean>
}

/**
 * createTranslateSettings 管理字幕翻译设置的运行时状态与后端持久化。
 * onError 用于把持久化/读取/检测错误交给现有错误提示。
 */
export function createTranslateSettings(
  api: TranslateSettingsApi,
  onError: (err: unknown) => void = () => {},
): TranslateSettingsStore {
  const settings = ref<TranslateSettings>(normalizeTranslateSettings(null))

  async function persist(next: TranslateSettings): Promise<void> {
    // 先更新界面值：持久化失败也保留用户看到的选择。
    settings.value = next
    try {
      await api.SetTranslateSettings({ ...next })
    } catch (err) {
      onError(err)
    }
  }

  async function load(): Promise<void> {
    try {
      settings.value = normalizeTranslateSettings(await api.GetTranslateSettings())
    } catch (err) {
      // 读取失败回退为默认，但不改坏已有运行时值之外的任何状态。
      settings.value = normalizeTranslateSettings(null)
      onError(err)
    }
  }

  async function setProvider(value: TranslateProvider): Promise<void> {
    await persist({ ...settings.value, Provider: value, Enabled: value !== 'off' })
  }

  async function setFreeProvider(value: FreeTranslateProvider): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, FreeProvider: value }))
  }

  async function setFreeAPIKey(value: string): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, FreeAPIKey: value }))
  }

  async function setAzureRegion(value: string): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, AzureRegion: value }))
  }

  async function setCFAccountID(value: string): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, CFAccountID: value }))
  }

  async function setCFToken(value: string): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, CFToken: value }))
  }

  async function checkModel(): Promise<boolean> {
    try {
      await api.CheckTranslateModel()
      return true
    } catch (err) {
      onError(err)
      return false
    }
  }

  return { settings, load, setProvider, setFreeProvider, setFreeAPIKey, setAzureRegion, setCFAccountID, setCFToken, checkModel }
}
