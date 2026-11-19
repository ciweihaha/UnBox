// subtitleTranslate.ts — 免费接口英译中：播放器字幕的翻译队列/去重/缓存。
//
// 只做英译中、单条短文本：PlaybackView 在字幕轨 cuechange 时把当前 cue
// 文本传进来，拿到译文就用覆盖层显示，拿不到（接口没通、超时、返回空）
// 就返回原文，翻译失败永远不挡播放。本地大模型已删除
// （2026-11-19 用户决定：效果不好），翻译只走免费接口。

/** 翻译开关二选一：免费接口 / 关闭。 */
export type TranslateProvider = 'free' | 'off'

export const TRANSLATE_PROVIDERS: TranslateProvider[] = ['free', 'off']

/** 免费接口服务商二选一：微软 Azure（默认）/ Cloudflare。 */
export type FreeTranslateProvider = 'azure' | 'cloudflare'

export const FREE_TRANSLATE_PROVIDERS: FreeTranslateProvider[] = ['azure', 'cloudflare']

/** 翻译缓存上限：字幕去重后条目有限，超了就清空重记，避免长片看下来无限膨胀。 */
export const MAX_TRANSLATE_CACHE_ENTRIES = 500

export interface TranslateSettings {
  Enabled: boolean
  Provider: TranslateProvider
  FreeProvider: FreeTranslateProvider
  FreeAPIKey: string
  AzureRegion: string
  CFAccountID: string
  CFToken: string
}

/** normalizeTranslateSettings 把后端/旧版返回的残缺设置归一化。
 * 旧版只有 Enabled（无 Provider）时按开关迁移：开→免费接口，关→关闭；
 * 旧版 Provider 是 ollama（本地模型已删）或 cloudflare（现为服务商）
 * 时迁到免费接口，cloudflare 保留为服务商。 */
export function normalizeTranslateSettings(
  value: Partial<TranslateSettings> | null | undefined,
): TranslateSettings {
  // 旧版 Provider 可能是已删除的 'ollama' 或已改为服务商的 'cloudflare'：
  // 前者迁到免费接口，后者迁到免费接口 + Cloudflare 服务商。
  const legacyProvider = value?.Provider as string | undefined
  const provider: TranslateProvider =
    legacyProvider === 'free' || legacyProvider === 'off'
      ? legacyProvider
      : legacyProvider === 'ollama' || legacyProvider === 'cloudflare'
        ? 'free'
        : value?.Enabled === true
          ? 'free'
          : 'off'
  let freeProvider: FreeTranslateProvider = 'azure'
  if (value?.FreeProvider === 'azure' || value?.FreeProvider === 'cloudflare') {
    freeProvider = value.FreeProvider
  } else if (legacyProvider === 'cloudflare') {
    freeProvider = 'cloudflare'
  }
  return {
    Enabled: provider !== 'off',
    Provider: provider,
    FreeProvider: freeProvider,
    FreeAPIKey: typeof value?.FreeAPIKey === 'string' ? value.FreeAPIKey.trim() : '',
    AzureRegion: typeof value?.AzureRegion === 'string' ? value.AzureRegion.trim() : '',
    CFAccountID: typeof value?.CFAccountID === 'string' ? value.CFAccountID.trim() : '',
    CFToken: typeof value?.CFToken === 'string' ? value.CFToken.trim() : '',
  }
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
        // 接口没通、超时、服务报错：回退原文，不缓存失败结果，下次再试。
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
