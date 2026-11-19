import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const playbackView = readFileSync('src/components/PlaybackView.vue', 'utf8')

// 停滞检测接线契约（v0.9.17）：直播冻住不报错、不缓冲时，唯一能发现的
// 信号就是进度不动。检测器必须在挂上播放计划时就启动——只在 onPlay 里
// 启动的话，流根本起不来（一直没 playing 事件）就永远没人管。
describe('playback stall wiring', () => {
  it('starts the stall detector when a plan is attached, not only on play', () => {
    expect(playbackView).toMatch(/async function attach[\s\S]*?ensureStallDetector\(\)\.start\(\)/)
    expect(playbackView).toContain("emit('stalled')")
  })

  it('restarts the detector on play and stops it on cleanup', () => {
    expect(playbackView).toMatch(/function onPlay[\s\S]*?ensureStallDetector\(\)\.start\(\)/)
    expect(playbackView).toMatch(/function cleanup[\s\S]*?stallDetector\?\.stop\(\)/)
  })

  it('feeds progress from timeupdate and suspends the clock while seeking', () => {
    expect(playbackView).toMatch(/function onTimeUpdate[\s\S]*?stallDetector\?\.progress\(element\.currentTime\)/)
    expect(playbackView).toMatch(/function onSeeked[\s\S]*?stallDetector\?\.setPlaying\(true\)/)
  })
})
