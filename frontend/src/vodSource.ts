// vodSource.ts — 点播源来源（本地 / Cloudflare）的展示与校验小工具。
//
// 后端方法见 internal/shell/vodsource.go：SetCloudflareVodURL、
// RefreshCloudflareVodSource、UseLocalVodSource、GetVodSourceStatus。

export type VodSourceKind = 'local' | 'cloudflare'

/** vodSourceKindLabel 把来源 key 翻成设置页显示的中文。 */
export function vodSourceKindLabel(kind: string): string {
  return kind === 'cloudflare' ? 'Cloudflare' : '本地'
}

/** isValidCfVodURL 校验 Cloudflare 点播源地址：必须是 http(s) 地址。 */
export function isValidCfVodURL(url: string): boolean {
  try {
    const u = new URL(url.trim())
    return u.protocol === 'http:' || u.protocol === 'https:'
  } catch {
    return false
  }
}
