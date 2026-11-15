// translateSettings.ts — 设置页「字幕翻译」分类的加载、持久化与模型检测。
//
// 开关默认关；模型与地址缺失/非法时回退默认。切换时先更新运行时值再持久化，
// 持久化失败保留运行时值并把错误交给调用方展示（与 playbackSettings.ts 同一模式）。
import { ref, type Ref } from 'vue'
import { normalizeTranslateSettings, type TranslateSettings } from './subtitleTranslate'

export type { TranslateSettings }

/** 后端设置接口的最小形状（对应 ShellService 的三个绑定方法）。 */
export interface TranslateSettingsApi {
  GetTranslateSettings(): Promise<Partial<TranslateSettings> | null | undefined>
  SetTranslateSettings(settings: TranslateSettings): Promise<unknown>
  CheckTranslateModel(): Promise<unknown>
}

export interface TranslateSettingsStore {
  settings: Ref<TranslateSettings>
  load: () => Promise<void>
  setEnabled: (value: boolean) => Promise<void>
  setModel: (value: string) => Promise<void>
  setEndpoint: (value: string) => Promise<void>
  /** checkModel 检测本地是否已拉取当前模型；就绪返回 true，否则 false。 */
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

  async function setEnabled(value: boolean): Promise<void> {
    await persist({ ...settings.value, Enabled: value === true })
  }

  async function setModel(value: string): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, Model: value }))
  }

  async function setEndpoint(value: string): Promise<void> {
    await persist(normalizeTranslateSettings({ ...settings.value, Endpoint: value }))
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

  return { settings, load, setEnabled, setModel, setEndpoint, checkModel }
}
