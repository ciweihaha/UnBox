import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')
const header = readFileSync('src/components/VodDetailHeader.vue', 'utf8')

// 详情页重做（v0.9.16）：原来是半透明浮层（背后首页透出来、边上还露着
// 首页内容），用户要求改成正经的独立详情页；布局也重排：上半部分左是
// 播放区、右是海报+介绍，下半部分是选集网格。
// 约定：整页 fixed 盖住首页（不透明底）、播放区左上、海报+介绍右上、
// 选集在下边整排、简介默认 clamp 3 行、「已完结」与收藏排成一行、
// 正在播放收进播放器上方状态行、剧集按钮保证「第01集」完整显示、
// 季切换 pills 与「更多类似内容」保持。
describe('vod detail redesign', () => {
  it('is a full page, not a floating layer: opaque fixed page covering home', () => {
    expect(app).toContain('vod-detail-page')
    expect(app).not.toContain('vod-detail-overlay')
    expect(app).not.toContain('vod-detail-modal')
    expect(stylesheet).toMatch(/\.vod-detail-page\s*\{[^}]*position:\s*fixed;/s)
    expect(stylesheet).toMatch(/\.vod-detail-page\s*\{[^}]*inset:\s*0;/s)
    // 不透明底色：背后首页一点都不能透出来。
    expect(stylesheet).toMatch(/\.vod-detail-page\s*\{[^}]*background:\s*var\(--bg\);/s)
    // 盖住顶部导航（z-index 60），否则导航还飘在详情上面。
    expect(stylesheet).toMatch(/\.vod-detail-page\s*\{[^}]*z-index:\s*100;/s)
    expect(stylesheet).not.toContain('.vod-detail-overlay')
    expect(stylesheet).not.toContain('.vod-detail-modal')
  })

  it('lays out the top as player left, poster plus intro right, episodes below', () => {
    expect(app).toContain('vod-detail-top')
    expect(app).toContain('vod-detail-player')
    expect(app).toContain('vod-detail-info')
    expect(app).toContain('vod-detail-episodes')
    // 播放区在左、信息区在右，选集在下面整排。
    expect(stylesheet).toMatch(/\.vod-detail-top\s*\{[^}]*display:\s*grid;/s)
    const top = app.indexOf('vod-detail-top')
    const player = app.indexOf('vod-detail-player', top)
    const info = app.indexOf('vod-detail-info', player)
    const eps = app.indexOf('vod-detail-episodes', info)
    expect(player).toBeGreaterThan(-1)
    expect(info).toBeGreaterThan(player)
    expect(eps).toBeGreaterThan(info)
    // 窄窗口上下叠放，播放区仍在上。
    expect(stylesheet).toMatch(/@media \(max-width: 900px\)[\s\S]*?\.vod-detail-top\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\);/s)
  })

  it('puts the poster in the info column on the right, 2:3', () => {
    expect(stylesheet).toMatch(/\.vod-detail-poster\s*\{[^}]*aspect-ratio:\s*2 \/ 3;/s)
    expect(app).toMatch(/vod-detail-info[\s\S]*?vod-detail-poster/)
  })

  it('clamps the synopsis to 3 lines by default', () => {
    expect(stylesheet).toMatch(/\.vod-detail \.desc\s*\{[^}]*-webkit-line-clamp:\s*3;/s)
  })

  it('puts the finished badge and the favorite button in one action row', () => {
    expect(app).toContain('<div class="vod-detail-actions">')
    expect(app).toMatch(/vod-detail-actions[\s\S]*?vod-detail-remarks/)
    expect(app).toMatch(/vod-detail-actions[\s\S]*?favorite-toggle/)
    expect(stylesheet).toMatch(/\.vod-detail-actions\s*\{[^}]*display:\s*flex;/s)
  })

  it('moves the now-playing line out of the modal header into a status line above the player', () => {
    expect(app).not.toContain('<VodDetailHeader :now-playing')
    expect(header).not.toContain('nowPlaying')
    expect(header).not.toContain('class="now"')
    expect(app).toContain('class="vod-now-playing"')
    expect(app).toContain('正在播放：{{ vodNowPlaying }}')
    expect(stylesheet).toMatch(/\.vod-now-playing\s*\{[^}]/s)
  })

  it('sizes episode buttons so that 第01集 shows in full instead of truncating', () => {
    expect(stylesheet).toMatch(/\.vod-detail-episodes \.ep-list\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(72px, 1fr\)\);/s)
    // 选集按钮做大（0.95rem）：小而素看不清，正在播的集除高亮外再加 ▶ 标记。
    expect(stylesheet).toMatch(/\.ep-list button\s*\{[^}]*font-size:\s*0\.95rem;/s)
    expect(stylesheet).toMatch(/\.ep-list button\.active::before\s*\{[^}]*content:\s*'▶ '/s)
  })

  it('keeps the season pills and the similar-content poster grid', () => {
    expect(app).toContain('ep-pagination')
    expect(app).toContain('更多类似内容')
    expect(app).toMatch(/vod-similar[\s\S]*?ContentCardList[\s\S]*?:style="'grid'"/)
  })

  it('keeps the back button as the way out of the full page', () => {
    expect(app).toContain('<VodDetailHeader @back="backFromVodDetail" />')
    expect(header).toContain('← 返回')
  })
})
