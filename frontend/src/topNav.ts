// topNav.ts — 顶部导航的点播分类归属。
//
// 点播页按当前分类名高亮对应的顶部分类；分类名是源站下发的，各源命名不一，
// 按包含匹配（「连续剧」「剧集」都算电视，「少儿」算儿童）。
// 顶部只放电影 / 电视 / 综艺 / 儿童四个分类入口，其余分类走首页快捷入口与搜索。

export type TopNavKey = 'home' | 'movies' | 'tv' | 'variety' | 'kids' | 'live' | 'favorites' | 'vod' | ''

export interface TopNavCategory {
  key: 'movies' | 'tv' | 'variety' | 'kids'
  label: string
  /** 按分类名找真实分类时的匹配词（首个为主名，其余为别名）。 */
  search: string[]
}

/** 顶部导航的四个点播分类入口（顺序即导航栏顺序）。 */
export const TOP_NAV_CATEGORIES: TopNavCategory[] = [
  { key: 'movies', label: '电影', search: ['电影'] },
  { key: 'tv', label: '电视', search: ['电视', '电视剧', '连续剧', '剧集'] },
  { key: 'variety', label: '综艺', search: ['综艺'] },
  { key: 'kids', label: '儿童', search: ['儿童', '少儿'] },
]

/** topNavKeyForCategory 把点播当前分类名归属到顶部分类；归属不上时回 'vod'。 */
export function topNavKeyForCategory(title: string): TopNavKey {
  if (title.includes('电影')) return 'movies'
  if (title.includes('电视') || title.includes('连续剧') || title.includes('剧集')) return 'tv'
  if (title.includes('综艺')) return 'variety'
  if (title.includes('儿童') || title.includes('少儿')) return 'kids'
  return 'vod'
}
