import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')
const cardList = readFileSync('src/components/ContentCardList.vue', 'utf8')
const cardItem = readFileSync('src/contentCardItem.ts', 'utf8')
const playbackView = readFileSync('src/components/PlaybackView.vue', 'utf8')

// Netflix 风格改版（v0.9.9）契约：近黑画布 + 顶部导航 + 全幅横幅 +
// 横向海报行 + 红色关键强调。行、横幅、弹窗的内容一律来自真实数据流，
// 预览里的 mock 影片绝不进仓库。
describe('netflix palette contract', () => {
  it('uses the netflix canvas, card surface and red accent as default theme', () => {
    expect(stylesheet).toMatch(/:root\s*\{[^}]*--bg:\s*#141414;/s)
    expect(stylesheet).toMatch(/:root\s*\{[^}]*--panel:\s*#181818;/s)
    expect(stylesheet).toMatch(/:root\s*\{[^}]*--text:\s*#ffffff;/s)
    expect(stylesheet).toMatch(/:root\s*\{[^}]*--text-dim:\s*#b3b3b3;/s)
    expect(stylesheet).toMatch(/:root\s*\{[^}]*--accent:\s*#e50914;/s)
  })

  it('keeps the shared layout tokens the old contracts pin', () => {
    expect(stylesheet).toMatch(/--content-col:\s*148px;/)
    expect(stylesheet).toMatch(/--content-gap:\s*0\.75rem;/)
    expect(stylesheet).toMatch(/--vod-rail:\s*128px;/)
  })
})

describe('top nav contract', () => {
  it('replaces the left sidebar with a top nav', () => {
    expect(app).not.toContain('<aside class="sidebar">')
    expect(app).not.toContain('sidebarCollapsed')
    expect(app).toContain('<header class="topnav"')
    for (const label of ['首页', '电影', '电视剧', '直播', '我的片单', '媒体库']) {
      expect(app).toContain(label)
    }
    expect(app).toMatch(/aria-label="搜索"/)
    expect(app).toMatch(/aria-label="设置"/)
  })

  it('nav overlays the hero transparent and turns solid on scroll', () => {
    expect(stylesheet).toMatch(/\.topnav\s*\{[^}]*position:\s*fixed;[^}]*background:\s*linear-gradient\(180deg, rgba\(0, 0, 0, 0\.72\)/s)
    expect(stylesheet).toMatch(/\.topnav\.solid\s*\{[^}]*background:\s*#141414;/s)
    expect(stylesheet).toMatch(/\.topnav \.logo\s*\{[^}]*color:\s*#e50914;/s)
  })

  it('movie/drama links jump to the matching real vod category', () => {
    expect(app).toContain('goVodCategoryByTitle')
    expect(app).toMatch(/goVodCategoryByTitle\('电影'/)
    expect(app).toMatch(/goVodCategoryByTitle\('电视剧'/)
  })
})

describe('billboard contract', () => {
  it('renders a full-bleed hero from the first real history item', () => {
    expect(app).toContain('v-if="heroItem" class="home-hero"')
    expect(stylesheet).toMatch(/\.home-hero\s*\{[^}]*min-height:\s*8\d?vh;/s)
    expect(app).toContain('home-hero-play')
    expect(app).toContain('home-hero-more')
    expect(app).toContain('resumeVod(heroItem)')
    // 更多信息进真实详情，不编造影片。
    expect(app).toContain('openHeroDetail')
    expect(app).not.toContain('霓虹深渊')
  })

  it('keeps the play button white and the hero title large', () => {
    expect(stylesheet).toMatch(/\.home-hero-play\s*\{[^}]*background:\s*#fff;/s)
    expect(stylesheet).toMatch(/\.home-hero-info h2\s*\{[^}]*font-size:\s*clamp\(/s)
  })

  it('degrades gracefully without history: no billboard, no fake title', () => {
    const hero = app.match(/<div v-if="heroItem" class="home-hero">[\s\S]*?<\/div>\n      <\/div>/)
    expect(hero).not.toBeNull()
  })
})

describe('poster row contract', () => {
  it('renders home rows as horizontal poster tracks', () => {
    expect(app).toContain('为你精选')
    expect(app).toContain('继续观看')
    expect(app).toContain('netflix-row')
    expect(stylesheet).toMatch(/\.netflix-row\.content-card-grid\s*\{[^}]*display:\s*flex;[^}]*overflow-x:\s*auto;/s)
    expect(stylesheet).toMatch(/\.netflix-row \.content-card\s*\{[^}]*flex:\s*0 0 auto;/s)
    // 悬停放大预览是改版的标志之一。
    expect(stylesheet).toMatch(/\.netflix-row \.content-card:hover \.content-card-poster\s*\{[^}]*transform:\s*scale\(1\./s)
  })

  it('keeps posters 2:3 with 8px corners', () => {
    expect(stylesheet).toMatch(/\.content-card-poster\s*\{[^}]*aspect-ratio:\s*2 \/ 3;/s)
    expect(stylesheet).toMatch(/\.content-card-poster\s*\{[^}]*border-radius:\s*8px;/s)
  })

  it('renders continue-watching as wide 16:9 cards with a red progress bar', () => {
    expect(app).toContain('netflix-row-wide')
    expect(stylesheet).toMatch(/\.netflix-row-wide \.content-card-poster\s*\{[^}]*aspect-ratio:\s*16 \/ 9;/s)
    expect(cardItem).toContain('progressRatio')
    expect(cardList).toContain('content-card-progressbar')
    expect(stylesheet).toMatch(/\.content-card-progressbar i\s*\{[^}]*background:\s*#e50914;/s)
  })

  it('renders a top-10 row with outlined numbers from the real list', () => {
    expect(app).toContain('netflix-top10')
    expect(app).toContain('Top 10')
    expect(stylesheet).toMatch(/\.top10-num\s*\{[^}]*-webkit-text-stroke:/s)
  })
})

describe('detail modal contract', () => {
  it('opens the vod detail as a modal overlay, not an inline page', () => {
    expect(app).toContain('vod-detail-overlay')
    expect(app).toContain('vod-detail-modal')
    expect(stylesheet).toMatch(/\.vod-detail-overlay\s*\{[^}]*position:\s*fixed;/s)
    expect(stylesheet).toMatch(/\.vod-detail-modal\s*\{[^}]*background:\s*#181818;/s)
  })

  it('shows more-like content from the same real list', () => {
    expect(app).toContain('更多类似内容')
    expect(app).toContain('similarItems')
  })
})

describe('search count contract', () => {
  it('shows how many real results matched', () => {
    expect(app).toContain('search-count')
    expect(app).toMatch(/vodSearchItems\.length/)
  })
})

describe('settings dark contract', () => {
  it('keeps settings dark (the light preview page was a mock flaw)', () => {
    expect(stylesheet).not.toMatch(/\.settings-page\s*\{[^}]*background:\s*#f3f3f3;/s)
    expect(stylesheet).toMatch(/\.settings-page\s*\{[^}]*background:\s*#141414;/s)
    expect(stylesheet).toMatch(/\.src-section\s*\{[^}]*background:\s*#181818;/s)
  })
})

describe('player red progress contract', () => {
  it('paints the played portion of the seek bar red', () => {
    expect(playbackView).toContain('--seek-fill')
    expect(stylesheet).toMatch(/\.ctrl-seek::-moz-range-progress\s*\{[^}]*background:\s*#e50914;/s)
    expect(stylesheet).toMatch(/\.ctrl-seek::-webkit-slider-runnable-track\s*\{[^}]*#e50914/s)
  })
})

describe('home rows contract', () => {
  it('sources home rows from the vod list, never from search results', () => {
    // 搜完回首页时 visibleVodItems 是搜索结果；行必须只取 vodListItems。
    expect(app).toMatch(/homeListCardItems\s*=\s*computed[^\n]*vodListItems\.value\.map\(vodCardItem\)/)
    expect(app).toMatch(/top10Items\s*=\s*computed\([^\n]*vodListItems\.value\.slice\(0,\s*10\)/)
    expect(app).not.toMatch(/featuredRowItems\s*=\s*computed\([^\n]*visibleVodItems/)
    expect(app).not.toMatch(/top10Items\s*=\s*computed\([^\n]*visibleVodItems/)
  })

  it('does not show the same ten items in both home rows', () => {
    // Top 10 取前 10 条，为你精选取第 11–20 条；点选精选行按 i + 10 取回原条目。
    expect(app).toMatch(/featuredRowItems\s*=\s*computed\([^\n]*slice\(10,\s*20\)/)
    expect(app).toContain('openVodDetail(vodListItems[i + 10])')
  })
})

describe('poster fallback contract', () => {
  it('shows the gradient fallback when a poster url fails to load, not an empty box', () => {
    // imgError 只隐藏坏图；失败状态必须让 v-else 的渐变封面渲染出来。
    expect(app).toContain('heroLogoFailed')
    expect(app).toMatch(/heroItem\.VodLogo && !heroLogoFailed/)
    expect(app).toContain('top10FailedLogos')
    expect(app).toMatch(/item\.Logo && !top10FailedLogos\.has\(/)
    expect(app).toContain('detailLogoFailed')
    expect(app).toMatch(/vodDetail\.Logo && !detailLogoFailed/)
    expect(cardList).toContain('failedLogos')
    expect(cardList).toMatch(/item\.logo && !failedLogos\.has\(item\.key\)/)
  })

  it('gives the detail backdrop a gradient fallback with the same blurred fill', () => {
    expect(app).toContain('vod-hero-bg-fallback')
    expect(stylesheet).toMatch(/\.vod-hero-bg-fallback\s*\{[^}]*filter:\s*blur\(24px\)/s)
  })
})
