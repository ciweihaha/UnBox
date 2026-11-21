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

/**
 * CfVodDirectorySource 是点播源目录里的一项：名字 + 地址。
 * sites 是后端验证出的支持格式点播站点数：>=0 表示已验证，-1/缺失表示
 * 未知（拉不到，没验证出来）；sites === 0 表示已证实没有可用站点，
 * 不进下拉框（resolveCfVodSources 会滤掉）。
 */
export interface CfVodDirectorySource {
  name: string
  url: string
  sites?: number
}

/** 默认点播源目录地址：用户在 Cloudflare 上维护的源清单。 */
export const DEFAULT_CF_VOD_DIRECTORY_URL = 'https://live.ciweihaha.win/vod-sources.json'

/**
 * 目录拉不到时（没网、地址不对、文件还没挂上）的内置源：
 * 就是目录文件里会列的那几个，先顶着用（名字与目录文件一致）。
 */
export const DEFAULT_CF_VOD_SOURCES: CfVodDirectorySource[] = [
  { name: '聚合万能源（44站合一·支持4K）', url: 'https://live.ciweihaha.win/vod.json' },
  { name: 'OrionTV 聚合（38站）', url: 'https://ysp.ciweihaha.win/agg' },
  { name: '亚洲点播', url: 'https://ysp.ciweihaha.win/oktv-asia.json' },
]

/** 下拉框里“手动输入”那一项的值（不是真实地址）。 */
export const CF_VOD_SOURCE_MANUAL = '__manual__'

/**
 * parseCfVodDirectory 解析点播源目录 JSON：接受 `{ sources: [...] }`
 * 或裸数组；只要带名字和 http(s) 地址的项，去重（按地址），其余丢弃。
 */
export function parseCfVodDirectory(value: unknown): CfVodDirectorySource[] {
  const list = Array.isArray(value)
    ? value
    : (typeof value === 'object' && value !== null && Array.isArray((value as { sources?: unknown }).sources)
      ? (value as { sources: unknown[] }).sources
      : undefined)
  if (!Array.isArray(list)) return []
  const seen = new Set<string>()
  const out: CfVodDirectorySource[] = []
  for (const item of list) {
    if (typeof item !== 'object' || item === null) continue
    const { name, url, sites } = item as { name?: unknown; url?: unknown; sites?: unknown }
    const cleanName = typeof name === 'string' ? name.trim() : ''
    const cleanUrl = typeof url === 'string' ? url.trim() : ''
    if (!cleanName || !isValidCfVodURL(cleanUrl) || seen.has(cleanUrl)) continue
    seen.add(cleanUrl)
    // 后端给的 sites（已验证的支持站点数）原样透传；缺失表示未知。
    const entry: CfVodDirectorySource = { name: cleanName, url: cleanUrl }
    if (typeof sites === 'number') entry.sites = sites
    out.push(entry)
  }
  return out
}

/**
 * resolveCfVodSources 合并目录源与当前正在用的地址：目录里的优先；
 * 当前地址不在目录里时追加一项“正在用的源”，免得下拉框选不中正在用的源；
 * 目录和当前地址都没有时用内置默认源。
 * 已证实没有可用站点（sites === 0）的目录源不显示；没验证出来
 * （sites 缺失或 -1）的保留——不断网时不让下拉框变空。
 */
export function resolveCfVodSources(
  dir: CfVodDirectorySource[],
  currentUrl: string,
): CfVodDirectorySource[] {
  const merged = dir.filter((s) => s.sites !== 0)
  const url = currentUrl.trim()
  if (url && isValidCfVodURL(url) && !merged.some((s) => s.url === url)) {
    merged.push({ name: '正在用的源', url })
  }
  return merged.length ? merged : [...DEFAULT_CF_VOD_SOURCES]
}
