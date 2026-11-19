// liveSubtitleSettings.ts — 设置页「直播实时字幕」分类的加载与持久化。
//
// 开关默认关；识别模型默认 tiny.en、窗口默认 1.5 秒，非法时回退默认。
// 切换时先更新运行时值再持久化，持久化失败保留运行时值并把错误
// 交给调用方展示（与 translateSettings.ts 同一模式）。
import { ref, type Ref } from 'vue'

/** 默认识别模型：tiny.en（英文，快）；base.en 更准但慢约一倍。 */
export const DEFAULT_LIVE_WHISPER_MODEL = 'tiny.en'

/** 默认窗口时长（秒）：1.5 秒一窗，端到端约 3 秒。 */
export const DEFAULT_LIVE_WINDOW_SECONDS = 1.5

export interface LiveSubtitleSettings {
  Enabled: boolean
  WhisperModel: string
  WindowSeconds: number
}

/** normalizeLiveSubtitleSettings 把后端/旧版返回的残缺设置归一化。 */
export function normalizeLiveSubtitleSettings(
  value: Partial<LiveSubtitleSettings> | null | undefined,
): LiveSubtitleSettings {
  const model =
    typeof value?.WhisperModel === 'string' && value.WhisperModel.trim() !== ''
      ? value.WhisperModel.trim()
      : DEFAULT_LIVE_WHISPER_MODEL
  const windowSeconds =
    typeof value?.WindowSeconds === 'number' &&
    Number.isFinite(value.WindowSeconds) &&
    value.WindowSeconds > 0 &&
    value.WindowSeconds <= 10
      ? value.WindowSeconds
      : DEFAULT_LIVE_WINDOW_SECONDS
  return { Enabled: value?.Enabled === true, WhisperModel: model, WindowSeconds: windowSeconds }
}

/** 后端设置接口的最小形状（对应 ShellService 的两个绑定方法）。 */
export interface LiveSubtitleSettingsApi {
  GetLiveSubtitleSettings(): Promise<Partial<LiveSubtitleSettings> | null | undefined>
  SetLiveSubtitleSettings(settings: LiveSubtitleSettings): Promise<unknown>
  LiveSubtitleStatus(): Promise<LiveSubtitleStatus | null | undefined>
}

export interface LiveSubtitleStatus {
  Running: boolean
  Enabled: boolean
  WhisperModel: string
  WindowSeconds: number
  WindowsProcessed: number
  WindowsDropped: number
  LastLatencyMs: number
  AvgLatencyMs: number
  LastSubtitle: string
  Error: string
}

/**
 * liveSubtitleStatusText 把字幕引擎状态翻成设置页能看懂的一句话。
 *
 * 开关只表示“已启用”，引擎只在播英文直播（网页播放）时才启动，
 * 所以“开着却未启动”是正常待机、不是故障——文案必须写清楚原因，
 * 不能一边显示开、一边只说“未启动”让用户糊涂。
 */
export function liveSubtitleStatusText(st: LiveSubtitleStatus | null): string {
  if (!st) return '直播字幕状态查不到，稍后再试'
  if (st.Running) {
    return `直播字幕运行中，最近延迟约 ${(st.LastLatencyMs / 1000).toFixed(1)} 秒`
  }
  if (st.Error) return `直播字幕异常：${st.Error}`
  if (!st.Enabled) return '直播字幕已关闭：打开上方开关后，看英文直播时自动启动'
  return '已开启，尚未启动：开始播英文直播（网页播放）后自动启动'
}

export interface LiveSubtitleSettingsStore {
  settings: Ref<LiveSubtitleSettings>
  load: () => Promise<void>
  setEnabled: (value: boolean) => Promise<void>
  setWhisperModel: (value: string) => Promise<void>
  setWindowSeconds: (value: number) => Promise<void>
  /** status 查运行状态（在跑、延迟、最近字幕），失败返回 null。 */
  status: () => Promise<LiveSubtitleStatus | null>
}

/**
 * createLiveSubtitleSettings 管理直播实时字幕设置的运行时状态与后端持久化。
 * onError 用于把持久化/读取/状态错误交给现有错误提示。
 */
export function createLiveSubtitleSettings(
  api: LiveSubtitleSettingsApi,
  onError: (err: unknown) => void = () => {},
): LiveSubtitleSettingsStore {
  const settings = ref<LiveSubtitleSettings>(normalizeLiveSubtitleSettings(null))

  async function persist(next: LiveSubtitleSettings): Promise<void> {
    settings.value = next
    try {
      await api.SetLiveSubtitleSettings({ ...next })
    } catch (err) {
      onError(err)
    }
  }

  async function load(): Promise<void> {
    try {
      settings.value = normalizeLiveSubtitleSettings(await api.GetLiveSubtitleSettings())
    } catch (err) {
      settings.value = normalizeLiveSubtitleSettings(null)
      onError(err)
    }
  }

  async function setEnabled(value: boolean): Promise<void> {
    await persist({ ...settings.value, Enabled: value === true })
  }

  async function setWhisperModel(value: string): Promise<void> {
    await persist(normalizeLiveSubtitleSettings({ ...settings.value, WhisperModel: value }))
  }

  async function setWindowSeconds(value: number): Promise<void> {
    await persist(normalizeLiveSubtitleSettings({ ...settings.value, WindowSeconds: value }))
  }

  async function status(): Promise<LiveSubtitleStatus | null> {
    try {
      const got = await api.LiveSubtitleStatus()
      return got ?? null
    } catch (err) {
      onError(err)
      return null
    }
  }

  return { settings, load, setEnabled, setWhisperModel, setWindowSeconds, status }
}
