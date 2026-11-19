import { describe, expect, it, vi } from 'vitest'
import { PlaybackStallDetector } from './playbackStall'

describe('PlaybackStallDetector', () => {
  it('进度不动超过阈值回调一次', () => {
    vi.useFakeTimers()
    try {
      const onStall = vi.fn()
      let now = 0
      const d = new PlaybackStallDetector(10_000, onStall, () => now)
      d.start()
      d.progress(1)
      now = 9_999
      vi.advanceTimersByTime(10_000)
      expect(onStall).not.toHaveBeenCalled()
      now = 10_000
      vi.advanceTimersByTime(2_000)
      expect(onStall).toHaveBeenCalledTimes(1)
      // 只回调一次：再等也不重复。
      now = 60_000
      vi.advanceTimersByTime(10_000)
      expect(onStall).toHaveBeenCalledTimes(1)
      d.stop()
    } finally {
      vi.useRealTimers()
    }
  })

  it('进度前进刷新时钟，不误判', () => {
    vi.useFakeTimers()
    try {
      const onStall = vi.fn()
      let now = 0
      const d = new PlaybackStallDetector(10_000, onStall, () => now)
      d.start()
      d.progress(1)
      now = 9_000
      d.progress(2)
      now = 18_000
      vi.advanceTimersByTime(10_000)
      expect(onStall).not.toHaveBeenCalled()
      d.stop()
    } finally {
      vi.useRealTimers()
    }
  })

  it('暂停时不计时，恢复后重计时', () => {
    vi.useFakeTimers()
    try {
      const onStall = vi.fn()
      let now = 0
      const d = new PlaybackStallDetector(10_000, onStall, () => now)
      d.start()
      d.progress(5)
      d.setPlaying(false)
      now = 60_000
      vi.advanceTimersByTime(60_000)
      expect(onStall).not.toHaveBeenCalled()
      d.setPlaying(true)
      now = 69_999
      vi.advanceTimersByTime(10_000)
      expect(onStall).not.toHaveBeenCalled()
      now = 70_000
      vi.advanceTimersByTime(2_000)
      expect(onStall).toHaveBeenCalledTimes(1)
      d.stop()
    } finally {
      vi.useRealTimers()
    }
  })

  it('stop 后不再回调', () => {
    vi.useFakeTimers()
    try {
      const onStall = vi.fn()
      let now = 0
      const d = new PlaybackStallDetector(10_000, onStall, () => now)
      d.start()
      d.stop()
      now = 60_000
      vi.advanceTimersByTime(60_000)
      expect(onStall).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('进度倒退不刷新时钟', () => {
    vi.useFakeTimers()
    try {
      const onStall = vi.fn()
      let now = 0
      const d = new PlaybackStallDetector(10_000, onStall, () => now)
      d.start()
      d.progress(10)
      now = 5_000
      d.progress(9)
      now = 10_000
      vi.advanceTimersByTime(2_000)
      expect(onStall).toHaveBeenCalledTimes(1)
      d.stop()
    } finally {
      vi.useRealTimers()
    }
  })
})
