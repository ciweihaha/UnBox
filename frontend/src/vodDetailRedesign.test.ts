import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')
const header = readFileSync('src/components/VodDetailHeader.vue', 'utf8')

// 详情弹窗重做（第 2 项）：首页「继续播放」进去的弹窗顶部大留白、简介被挤、
// 红字「正在播放」飘在顶部、剧集按钮「第01…」被截断、整体比例失衡。
// 约定：弹窗限宽 1080px 居中、海报 2:3 约 200px、简介默认 clamp 3 行、
// 「已完结」与收藏排成一行操作区、正在播放收进播放器上方状态行、
// 剧集按钮自适应列保证「第01集」完整显示；季切换 pills 与「更多类似内容」保持。
describe('vod detail redesign', () => {
  it('limits the modal to about 1080px and centers it', () => {
    expect(stylesheet).toMatch(/\.vod-detail-modal\s*\{[^}]*max-width:\s*1080px;/s)
    expect(stylesheet).toMatch(/\.vod-detail-modal\s*\{[^}]*margin:\s*0 auto;/s)
  })

  it('sizes the hero poster to 2:3 at about 200px and lets the info column fill the hero height', () => {
    expect(stylesheet).toMatch(/\.vod-hero-poster\s*\{[^}]*width:\s*200px;/s)
    expect(stylesheet).toMatch(/\.vod-hero-poster\s*\{[^}]*aspect-ratio:\s*2 \/ 3;/s)
    expect(stylesheet).toMatch(/\.vod-hero-body\s*\{[^}]*align-items:\s*stretch;/s)
    expect(stylesheet).toMatch(/\.vod-hero-info\s*\{[^}]*display:\s*flex;/s)
    expect(stylesheet).toMatch(/\.vod-hero-info\s*\{[^}]*flex-direction:\s*column;/s)
  })

  it('clamps the synopsis to 3 lines by default', () => {
    expect(stylesheet).toMatch(/\.vod-detail \.desc\s*\{[^}]*-webkit-line-clamp:\s*3;/s)
  })

  it('puts the finished badge and the favorite button in one action row', () => {
    expect(app).toContain('<div class="vod-hero-actions">')
    expect(app).toMatch(/vod-hero-actions[\s\S]*?vod-hero-remarks/)
    expect(app).toMatch(/vod-hero-actions[\s\S]*?favorite-toggle/)
    expect(stylesheet).toMatch(/\.vod-hero-actions\s*\{[^}]*display:\s*flex;/s)
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
    expect(stylesheet).toMatch(/\.vod-episodes \.ep-list\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(72px, 1fr\)\);/s)
    expect(stylesheet).toMatch(/\.ep-list button\s*\{[^}]*font-size:\s*0\.8rem;/s)
  })

  it('keeps the season pills and the similar-content poster grid', () => {
    expect(app).toContain('ep-pagination')
    expect(app).toContain('更多类似内容')
    expect(app).toMatch(/vod-similar[\s\S]*?ContentCardList[\s\S]*?:style="'grid'"/)
  })
})
