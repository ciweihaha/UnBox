// wheelScroll.ts — 滚轮兜底：WebView2 里滚轮偶尔不滚动时手动补滚。
//
// 首页、分类页、详情页、直播列表的滚轮在云电脑上全都不动（只有键盘 PGDN
// 能滚）：容器都是 overflow-y:auto，原生滚动本应工作。原因未定位
// （可能是合成输入事件未被 WebView2 完整接收，也可能与输入焦点有关），
// 这里加一道兜底：滚轮事件后 80ms 检查目标容器动没动，没动就手动滚。
// 原生滚动正常时 80ms 内 scrollTop 已变，兜底不触发，不会双倍滚动；
// 滚轮事件根本到不了页面时兜底也不触发（那是原生层问题，需另外定位）。
// 输入框/下拉框/横向滚动行不接管。

/** 滚轮后多久检查原生滚动是否生效。 */
export const WHEEL_FALLBACK_DELAY_MS = 80

/** findVerticalScroller 从目标往上找第一个能纵向滚动的祖先容器。 */
export function findVerticalScroller(target: EventTarget | null): HTMLElement | null {
  let el = target as HTMLElement | null
  while (el) {
    if (el instanceof HTMLElement) {
      const style = window.getComputedStyle(el)
      const overflowY = style.overflowY
      if (
        (overflowY === 'auto' || overflowY === 'scroll') &&
        el.scrollHeight > el.clientHeight + 1
      ) {
        return el
      }
    }
    el = el.parentElement
  }
  return null
}

function canScroll(scroller: HTMLElement, deltaY: number): boolean {
  if (deltaY > 0) return scroller.scrollTop < scroller.scrollHeight - scroller.clientHeight - 1
  if (deltaY < 0) return scroller.scrollTop > 1
  return false
}

function isEditable(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el || !(el instanceof HTMLElement)) return false
  const tag = el.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable
}

/**
 * installWheelScrollFallback 在 root 上挂滚轮兜底：滚轮事件后检查目标容器
 * 是否滚动，未滚动且能朝该方向滚时手动 scrollBy 补一次。
 * 返回卸载函数（组件卸载时调用）。
 */
export function installWheelScrollFallback(root: HTMLElement): () => void {
  const onWheel = (event: WheelEvent) => {
    if (isEditable(event.target) || event.deltaY === 0) return
    const scroller = findVerticalScroller(event.target)
    if (!scroller || !canScroll(scroller, event.deltaY)) return
    const before = scroller.scrollTop
    const deltaY = event.deltaY
    window.setTimeout(() => {
      if (scroller.scrollTop === before && canScroll(scroller, deltaY)) {
        scroller.scrollBy({ top: deltaY, behavior: 'auto' })
      }
    }, WHEEL_FALLBACK_DELAY_MS)
  }
  root.addEventListener('wheel', onWheel, { passive: true })
  return () => root.removeEventListener('wheel', onWheel)
}
