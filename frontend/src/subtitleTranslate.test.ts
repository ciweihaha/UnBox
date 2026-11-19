import { describe, expect, it, vi } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import {
  activeCueText,
  createSubtitleTranslator,
  isTranslatableText,
  normalizeTranslateSettings,
  type TranslateSubtitleApi,
} from './subtitleTranslate'

const playbackView = readFileSync('src/components/PlaybackView.vue', 'utf8')

function api(translate: TranslateSubtitleApi['TranslateSubtitle']): TranslateSubtitleApi {
  return { TranslateSubtitle: translate }
}

describe('isTranslatableText', () => {
  it('英文短句可翻译', () => {
    expect(isTranslatableText('Hello, world!')).toBe(true)
    expect(isTranslatableText("I'm fine.")).toBe(true)
  })

  it('空文本、纯标点、纯数字不翻译', () => {
    expect(isTranslatableText('')).toBe(false)
    expect(isTranslatableText('   ')).toBe(false)
    expect(isTranslatableText('...')).toBe(false)
    expect(isTranslatableText('123')).toBe(false)
  })

  it('含中文不翻译（已是中文或双语字幕直接显示原文）', () => {
    expect(isTranslatableText('你好')).toBe(false)
    expect(isTranslatableText('Hello 你好')).toBe(false)
  })
})

describe('normalizeTranslateSettings', () => {
  it('缺失值回退默认：开关关、免费接口、Azure 服务商', () => {
    expect(normalizeTranslateSettings(null)).toEqual({
      Enabled: false,
      Provider: 'off',
      FreeProvider: 'azure',
      FreeAPIKey: '',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
    })
    expect(normalizeTranslateSettings({})).toEqual({
      Enabled: false,
      Provider: 'off',
      FreeProvider: 'azure',
      FreeAPIKey: '',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
    })
  })

  it('只有 Enabled 严格等于 true 才算开；Key 去空白', () => {
    expect(
      normalizeTranslateSettings({ Enabled: 'yes', FreeAPIKey: '  key-1  ' } as never),
    ).toEqual({
      Enabled: false,
      Provider: 'off',
      FreeProvider: 'azure',
      FreeAPIKey: 'key-1',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
    })
  })

  it('旧版 Provider 迁移：ollama 迁到免费接口，cloudflare 迁到免费接口 + Cloudflare 服务商', () => {
    expect(normalizeTranslateSettings({ Enabled: true, Provider: 'ollama' } as never)).toEqual({
      Enabled: true,
      Provider: 'free',
      FreeProvider: 'azure',
      FreeAPIKey: '',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
    })
    expect(normalizeTranslateSettings({ Enabled: true, Provider: 'cloudflare' } as never)).toEqual({
      Enabled: true,
      Provider: 'free',
      FreeProvider: 'cloudflare',
      FreeAPIKey: '',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
    })
  })

  it('免费接口 + 服务商 + Key 原样保留', () => {
    expect(
      normalizeTranslateSettings({
        Enabled: true,
        Provider: 'free',
        FreeProvider: 'google',
        FreeAPIKey: 'google-key-1',
      }),
    ).toEqual({
      Enabled: true,
      Provider: 'free',
      FreeProvider: 'google',
      FreeAPIKey: 'google-key-1',
      AzureRegion: '',
      CFAccountID: '',
      CFToken: '',
    })
  })
})

describe('activeCueText', () => {
  it('无轨道或无活动 cue 返回空串', () => {
    expect(activeCueText(null)).toBe('')
    expect(activeCueText(undefined)).toBe('')
    expect(activeCueText({ activeCues: null } as never)).toBe('')
    expect(activeCueText({ activeCues: [] } as never)).toBe('')
  })

  it('取第一条活动 cue 的文本，去 VTT 标签与首尾空白', () => {
    expect(activeCueText({ activeCues: [{ text: '  <i>Hello</i>  ' }] } as never)).toBe('Hello')
  })
})

describe('createSubtitleTranslator', () => {
  it('英文调后端翻译并返回译文；同一条只调一次（缓存）', async () => {
    const translate = vi.fn(async (text: string) => `译:${text}`)
    const translator = createSubtitleTranslator(api(translate))
    expect(await translator.translate('Hello')).toBe('译:Hello')
    expect(await translator.translate('Hello')).toBe('译:Hello')
    expect(translate).toHaveBeenCalledTimes(1)
  })

  it('并发翻译同一条只发一次请求（在途去重）', async () => {
    let calls = 0
    const translate = vi.fn(async (text: string) => {
      calls++
      await new Promise((r) => setTimeout(r, 10))
      return `译:${text}`
    })
    const translator = createSubtitleTranslator(api(translate))
    const [a, b] = await Promise.all([translator.translate('Hello'), translator.translate('Hello')])
    expect(a).toBe('译:Hello')
    expect(b).toBe('译:Hello')
    expect(calls).toBe(1)
  })

  it('中文与空文本不发请求、直接返回原文', async () => {
    const translate = vi.fn(async (text: string) => `译:${text}`)
    const translator = createSubtitleTranslator(api(translate))
    expect(await translator.translate('你好')).toBe('你好')
    expect(await translator.translate('  ')).toBe('  ')
    expect(translate).not.toHaveBeenCalled()
  })

  it('后端失败返回原文、不抛错（翻译失败绝不挡播放）', async () => {
    const translator = createSubtitleTranslator(api(async () => {
      throw new Error('接口超时')
    }))
    await expect(translator.translate('Hello')).resolves.toBe('Hello')
  })

  it('后端返回空译文时回退原文', async () => {
    const translator = createSubtitleTranslator(api(async () => '   '))
    await expect(translator.translate('Hello')).resolves.toBe('Hello')
  })

  it('缓存满了清空重记：超量后最早那条重新翻译', async () => {
    const translate = vi.fn(async (text: string) => `译:${text}`)
    const translator = createSubtitleTranslator(api(translate))
    for (let i = 0; i < 501; i++) await translator.translate(`Line ${i}`)
    expect(translate).toHaveBeenCalledTimes(501)
    await translator.translate('Line 0')
    expect(translate).toHaveBeenCalledTimes(502)
  })

  it('clearCache 后重新翻译', async () => {
    const translate = vi.fn(async (text: string) => `译:${text}`)
    const translator = createSubtitleTranslator(api(translate))
    await translator.translate('Hello')
    translator.clearCache()
    await translator.translate('Hello')
    expect(translate).toHaveBeenCalledTimes(2)
  })
})

describe('播放器字幕翻译接线', () => {
  it('PlaybackView 监听字幕轨 cuechange，翻译后用覆盖层显示译文', () => {
    expect(playbackView).toContain('cuechange')
    expect(playbackView).toContain('translatedSubtitle')
    expect(playbackView).toContain('subtitle-translation')
    expect(playbackView).toContain('translateEnabled')
    expect(playbackView).toContain('translateText')
  })

  it('翻译开启时原生字幕轨设 hidden 防重影，关闭时恢复', () => {
    expect(playbackView).toContain("'hidden'")
    expect(playbackView).toContain("'showing'")
  })

  it('后到字幕轨走 hls.js SUBTITLE_TRACKS_UPDATED，不用 TextTrackList.addEventListener（jsdom 没有它）', () => {
    expect(playbackView).toContain('SUBTITLE_TRACKS_UPDATED')
    expect(playbackView).not.toContain('textTracks.addEventListener')
  })
})
