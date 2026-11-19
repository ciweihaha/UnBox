import { describe, expect, it, vi } from 'vitest'
import { findVerticalScroller, installWheelScrollFallback, WHEEL_FALLBACK_DELAY_MS } from './wheelScroll'

function makeScroller(scrollHeight: number, clientHeight: number): HTMLElement & { scrollByMock: ReturnType<typeof vi.fn> } {
  const el = document.createElement('div') as unknown as HTMLElement & { scrollByMock: ReturnType<typeof vi.fn> }
  Object.defineProperty(el, 'scrollHeight', { value: scrollHeight, configurable: true })
  Object.defineProperty(el, 'clientHeight', { value: clientHeight, configurable: true })
  el.style.overflowY = 'auto'
  // jsdom 没有实现 Element.scrollBy，测试里挂一个 mock 供断言。
  el.scrollByMock = vi.fn()
  el.scrollBy = el.scrollByMock as unknown as HTMLElement['scrollBy']
  document.body.appendChild(el)
  return el
}

describe('findVerticalScroller', () => {
  it('找到能纵向滚动的祖先', () => {
    const outer = makeScroller(1000, 300)
    const inner = document.createElement('div')
    outer.appendChild(inner)
    const leaf = document.createElement('span')
    inner.appendChild(leaf)
    expect(findVerticalScroller(leaf)).toBe(outer)
    outer.remove()
  })

  it('内容没溢出的容器不算', () => {
    const outer = makeScroller(300, 300)
    const leaf = document.createElement('span')
    outer.appendChild(leaf)
    expect(findVerticalScroller(leaf)).toBeNull()
    outer.remove()
  })

  it('横向滚动行不算纵向容器', () => {
    const row = makeScroller(100, 100)
    row.style.overflowY = 'hidden'
    Object.defineProperty(row, 'scrollWidth', { value: 2000, configurable: true })
    Object.defineProperty(row, 'clientWidth', { value: 500, configurable: true })
    const leaf = document.createElement('span')
    row.appendChild(leaf)
    expect(findVerticalScroller(leaf)).toBeNull()
    row.remove()
  })
})

describe('installWheelScrollFallback', () => {
  it('原生滚动生效时不补滚', () => {
    vi.useFakeTimers()
    try {
      const root = makeScroller(1000, 300)
      const uninstall = installWheelScrollFallback(root)
      root.dispatchEvent(new WheelEvent('wheel', { deltaY: 120, bubbles: true }))
      // 模拟原生滚动已发生：scrollTop 变了。
      Object.defineProperty(root, 'scrollTop', { value: 120, configurable: true })
      vi.advanceTimersByTime(WHEEL_FALLBACK_DELAY_MS + 10)
      expect(root.scrollByMock).not.toHaveBeenCalled()
      uninstall()
      root.remove()
    } finally {
      vi.useRealTimers()
    }
  })

  it('原生滚动没动时手动补滚一次', () => {
    vi.useFakeTimers()
    try {
      const root = makeScroller(1000, 300)
      const uninstall = installWheelScrollFallback(root)
      root.dispatchEvent(new WheelEvent('wheel', { deltaY: 120, bubbles: true }))
      vi.advanceTimersByTime(WHEEL_FALLBACK_DELAY_MS + 10)
      expect(root.scrollByMock).toHaveBeenCalledTimes(1)
      expect(root.scrollByMock).toHaveBeenCalledWith({ top: 120, behavior: 'auto' })
      uninstall()
      root.remove()
    } finally {
      vi.useRealTimers()
    }
  })

  it('已到底/顶时不补滚', () => {
    vi.useFakeTimers()
    try {
      const root = makeScroller(1000, 300)
      Object.defineProperty(root, 'scrollTop', { value: 700, configurable: true })
      const uninstall = installWheelScrollFallback(root)
      root.dispatchEvent(new WheelEvent('wheel', { deltaY: 120, bubbles: true }))
      vi.advanceTimersByTime(WHEEL_FALLBACK_DELAY_MS + 10)
      expect(root.scrollByMock).not.toHaveBeenCalled()
      uninstall()
      root.remove()
    } finally {
      vi.useRealTimers()
    }
  })
})
