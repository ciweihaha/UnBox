// subtitleTranslate.ts — 本地大模型英译中：播放器字幕的翻译队列/去重/缓存。
//
// 只做英译中、单条短文本：PlaybackView 在字幕轨 cuechange 时把当前 cue
// 文本传进来，拿到译文就用覆盖层显示，拿不到（模型没装、服务没起、超时、
// 返回空）就返回原文，翻译失败永远不挡播放。

/** 默认翻译模型：Qwen2.5-3B 指令版，下载约 1.9GB，字幕短句在 GPU 上约 0.5 秒/条。 */
export const DEFAULT_TRANSLATE_MODEL = 'qwen2.5:3b'

/** 默认本地模型地址：Ollama 默认监听地址。 */
export const DEFAULT_TRANSLATE_ENDPOINT = 'http://127.0.0.1:11434'

/** 翻译缓存上限：字幕去重后条目有限，超了就清空重记，避免长片看下来无限膨胀。 */
export const MAX_TRANSLATE_CACHE_ENTRIES = 500

export interface TranslateSettings {
  Enabled: boolean
  Model: string
  Endpoint: string
}

/** normalizeTranslateSettings 把后端/旧版返回的残缺设置归一化；空模型/地址回退默认。 */
export function normalizeTranslateSettings(
  value: Partial<TranslateSettings> | null | undefined,
): TranslateSettings {
  const model =
    typeof value?.Model === 'string' && value.Model.trim() !== ''
      ? value.Model.trim()
      : DEFAULT_TRANSLATE_MODEL
  const endpoint =
    typeof value?.Endpoint === 'string' && value.Endpoint.trim() !== ''
      ? value.Endpoint.trim()
      : DEFAULT_TRANSLATE_ENDPOINT
  return { Enabled: value?.Enabled === true, Model: model, Endpoint: endpoint }
}

/** isTranslatableText：英文短句才值得翻译；含中文、空文本、纯标点数字直接跳过。 */
export function isTranslatableText(text: string): boolean {
  if (text.trim() === '') return false
  // 含中文就不翻译（已是中文或双语字幕直接显示原文）。
  if (/[\u4e00-\u9fff]/.test(text)) return false
  return /[A-Za-z]/.test(text)
}

/** activeCueText 取字幕轨当前活动 cue 的文本；VTT 行内标签去掉。 */
export function activeCueText(track: TextTrack | null | undefined): string {
  // TextTrackCue 基类没有 text，只有 VTTCue 有：用交叉类型取文本，缺失按空串处理。
  const cue = track?.activeCues?.[0] as (TextTrackCue & { text?: string }) | undefined
  const raw = typeof cue?.text === 'string' ? cue.text : ''
  return raw.replace(/<[^>]*>/g, '').trim()
}

/** 后端翻译接口的最小形状（对应 ShellService.TranslateSubtitle）。 */
export interface TranslateSubtitleApi {
  TranslateSubtitle(text: string): Promise<string>
}

export interface SubtitleTranslator {
  /** 翻译一条字幕：跳过/失败都返回原文，永不抛错。 */
  translate(text: string): Promise<string>
  clearCache(): void
}

/**
 * createSubtitleTranslator 管理翻译请求：同一条只翻一次（缓存），
 * 并发同一条只发一次请求（在途去重），失败回退原文且不缓存失败结果。
 */
export function createSubtitleTranslator(api: TranslateSubtitleApi): SubtitleTranslator {
  const cache = new Map<string, string>()
  const inflight = new Map<string, Promise<string>>()

  async function translate(text: string): Promise<string> {
    if (!isTranslatableText(text)) return text
    const cached = cache.get(text)
    if (cached !== undefined) return cached
    const pending = inflight.get(text)
    if (pending) return pending
    const job = (async () => {
      try {
        const out = (await api.TranslateSubtitle(text)).trim()
        const result = out === '' ? text : out
        if (cache.size >= MAX_TRANSLATE_CACHE_ENTRIES) cache.clear()
        cache.set(text, result)
        return result
      } catch {
        // 模型没起、超时、服务报错：回退原文，不缓存失败结果，下次再试。
        return text
      } finally {
        inflight.delete(text)
      }
    })()
    inflight.set(text, job)
    return job
  }

  function clearCache(): void {
    cache.clear()
  }

  return { translate, clearCache }
}
