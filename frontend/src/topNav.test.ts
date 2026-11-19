import { describe, expect, it } from 'vitest'
import { TOP_NAV_CATEGORIES, topNavKeyForCategory } from './topNav'

describe('top nav categories', () => {
  it('顶部导航只有首页、电影、电视、综艺、儿童、直播、我的片单（无媒体库）', () => {
    expect(TOP_NAV_CATEGORIES.map(c => c.label)).toEqual(['电影', '电视', '综艺', '儿童'])
    expect(TOP_NAV_CATEGORIES.map(c => c.key)).toEqual(['movies', 'tv', 'variety', 'kids'])
  })

  it('按分类名归属高亮：别名也算', () => {
    expect(topNavKeyForCategory('电影')).toBe('movies')
    expect(topNavKeyForCategory('电影片')).toBe('movies')
    expect(topNavKeyForCategory('电视剧')).toBe('tv')
    expect(topNavKeyForCategory('电视')).toBe('tv')
    expect(topNavKeyForCategory('连续剧')).toBe('tv')
    expect(topNavKeyForCategory('剧集')).toBe('tv')
    expect(topNavKeyForCategory('综艺')).toBe('variety')
    expect(topNavKeyForCategory('儿童')).toBe('kids')
    expect(topNavKeyForCategory('少儿')).toBe('kids')
    expect(topNavKeyForCategory('动漫')).toBe('vod')
    expect(topNavKeyForCategory('')).toBe('vod')
  })
})
