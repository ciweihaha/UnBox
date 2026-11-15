import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { inflateSync } from 'node:zlib'

// 新图标（第 12 项）：播放主题、UnBox 风格、简洁现代，取代旧的三面三色
// 眯眯眼笑脸盒子。源头是 build/appicon.svg；各格式由它渲染：
// appicon.png（1024）、windows/icon.ico（6 档 PNG）、darwin/icons.icns
// （8 个 PNG 元素）、前端 public/appicon.png（与 build 版同字节）。
// macOS 的 Assets.car 只能在 macOS 上由 iconcomposer 从 appicon.icon 生成，
// 发版时 macOS runner 会从更新后的 wails_icon_vector.svg 重新生成。
// 注意：项目不装 @types/node，本文件只用 Uint8Array + 自写读数，不碰 Buffer。
const svg = readFileSync('../build/appicon.svg', 'utf8')
const appiconPng = readFileSync('../build/appicon.png')
const frontendPng = readFileSync('public/appicon.png')
const ico = readFileSync('../build/windows/icon.ico')
const icns = readFileSync('../build/darwin/icons.icns')
const macVector = readFileSync('../build/appicon.icon/Assets/wails_icon_vector.svg', 'utf8')
const indexHtml = readFileSync('index.html', 'utf8')

// 大端读 4 字节无符号整数。
function u32be(buf: Uint8Array, off: number): number {
  return ((buf[off] << 24) | (buf[off + 1] << 16) | (buf[off + 2] << 8) | buf[off + 3]) >>> 0
}

function pngSize(buf: Uint8Array): [number, number] {
  // PNG 签名 8 字节 + IHDR 长度 4 + 'IHDR' 4，宽高各 4 字节大端。
  expect(Array.from(buf.subarray(0, 4))).toEqual([0x89, 0x50, 0x4e, 0x47])
  return [u32be(buf, 16), u32be(buf, 20)]
}

// 取 PNG 里某像素的 RGBA。只支持 8-bit RGBA（color type 6），
// 本项目渲染产物（Chromium 截屏 / PIL 缩放）就是这个格式。
// 旧图中心是薄荷绿、顶部是米黄；新图中心是白色播放三角、顶部是红底，
// 像素断言能证明各格式文件确实由新 SVG 渲染，而不只是尺寸对得上。
function pngPixel(buf: Uint8Array, x: number, y: number): [number, number, number, number] {
  const width = u32be(buf, 16)
  expect(buf[24]).toBe(8)
  expect(buf[25]).toBe(6)
  const chunks: Uint8Array[] = []
  let total = 0
  let off = 8
  while (off < buf.length) {
    const len = u32be(buf, off)
    const type = String.fromCharCode(buf[off + 4], buf[off + 5], buf[off + 6], buf[off + 7])
    if (type === 'IDAT') {
      chunks.push(buf.subarray(off + 8, off + 8 + len))
      total += len
    }
    off += 12 + len
  }
  const idat = new Uint8Array(total)
  let pos = 0
  for (const c of chunks) {
    idat.set(c, pos)
    pos += c.length
  }
  const raw: Uint8Array = inflateSync(idat)
  const stride = width * 4
  let prev = new Uint8Array(stride)
  let rowStart = 0
  for (let r = 0; r <= y; r++) {
    const filter = raw[rowStart]
    const row = raw.subarray(rowStart + 1, rowStart + 1 + stride)
    const out = new Uint8Array(stride)
    for (let i = 0; i < stride; i++) {
      const a = i >= 4 ? out[i - 4] : 0
      const b = prev[i]
      const c = i >= 4 ? prev[i - 4] : 0
      let pred = 0
      if (filter === 1) pred = a
      else if (filter === 2) pred = b
      else if (filter === 3) pred = (a + b) >> 1
      else if (filter === 4) {
        const p = a + b - c
        const pa = Math.abs(p - a)
        const pb = Math.abs(p - b)
        const pc = Math.abs(p - c)
        pred = pa <= pb && pa <= pc ? a : pb <= pc ? b : c
      }
      out[i] = (row[i] + pred) & 0xff
    }
    prev = out
    rowStart += 1 + stride
  }
  const i = x * 4
  return [prev[i], prev[i + 1], prev[i + 2], prev[i + 3]]
}

describe('app icon', () => {
  it('replaces the old three-face smiley box with a play mark', () => {
    expect(svg).toContain('viewBox="0 0 1024 1024"')
    expect(svg).toContain('id="play"')
    expect(svg).toContain('linearGradient')
    // 旧图标的三面三色与眯眯眼笑脸（matrix 变换）必须消失。
    expect(svg).not.toContain('#F7E7B4')
    expect(svg).not.toContain('#A8D8C5')
    expect(svg).not.toContain('#A9C9E8')
    expect(svg).not.toContain('matrix(381')
  })

  it('renders appicon.png at 1024x1024 and keeps the frontend copy identical', () => {
    expect(pngSize(appiconPng)).toEqual([1024, 1024])
    expect(frontendPng.equals(appiconPng)).toBe(true)
    // 中心是白色播放三角，顶部是红底，圆角外是透明。
    expect(pngPixel(appiconPng, 512, 512)).toEqual([255, 255, 255, 255])
    const [r, g, b, a] = pngPixel(appiconPng, 512, 100)
    expect(r).toBeGreaterThan(200)
    expect(g).toBeLessThan(100)
    expect(b).toBeLessThan(100)
    expect(a).toBe(255)
    expect(pngPixel(appiconPng, 8, 8)).toEqual([0, 0, 0, 0])
  })

  it('keeps the macOS icon-composer vector identical to the source svg', () => {
    expect(macVector).toBe(svg)
  })

  it('builds windows/icon.ico with six PNG entries', () => {
    expect(ico[0] | (ico[1] << 8)).toBe(0)
    expect(ico[2] | (ico[3] << 8)).toBe(1)
    const count = ico[4] | (ico[5] << 8)
    expect(count).toBe(6)
    const sizes: number[] = []
    for (let i = 0; i < count; i++) {
      const off = 6 + i * 16
      const w = ico[off] || 256
      const size = ico[off + 8] | (ico[off + 9] << 8) | (ico[off + 10] << 16) | (ico[off + 11] << 24)
      const dataOff = ico[off + 12] | (ico[off + 13] << 8) | (ico[off + 14] << 16) | (ico[off + 15] << 24)
      expect(Array.from(ico.subarray(dataOff, dataOff + 4))).toEqual([0x89, 0x50, 0x4e, 0x47])
      sizes.push(w)
      expect(size).toBeGreaterThan(0)
    }
    expect(sizes).toEqual([256, 128, 64, 48, 32, 16])
    // 256 档中心是白色播放三角（确实由新 SVG 渲染）。
    const size0 = ico[14] | (ico[15] << 8) | (ico[16] << 16) | (ico[17] << 24)
    const dataOff0 = ico[18] | (ico[19] << 8) | (ico[20] << 16) | (ico[21] << 24)
    const png256 = ico.subarray(dataOff0, dataOff0 + size0)
    expect(pngPixel(png256, 128, 128)).toEqual([255, 255, 255, 255])
  })

  it('builds darwin/icons.icns with the same eight PNG elements', () => {
    expect(String.fromCharCode(icns[0], icns[1], icns[2], icns[3])).toBe('icns')
    expect(u32be(icns, 4)).toBe(icns.length)
    const types: string[] = []
    let off = 8
    while (off < icns.length) {
      const type = String.fromCharCode(icns[off], icns[off + 1], icns[off + 2], icns[off + 3])
      const len = u32be(icns, off + 4)
      expect(Array.from(icns.subarray(off + 8, off + 12))).toEqual([0x89, 0x50, 0x4e, 0x47])
      types.push(type)
      off += len
    }
    expect(off).toBe(icns.length)
    expect(types).toEqual(['ic10', 'ic14', 'ic09', 'ic13', 'ic08', 'ic07', 'ic12', 'ic11'])
    // ic10（1024 档）中心是白色播放三角（确实由新 SVG 渲染）。
    expect(pngPixel(icns.subarray(16, 8 + u32be(icns, 12)), 512, 512)).toEqual([255, 255, 255, 255])
  })

  it('links the frontend favicon to the new app icon', () => {
    expect(indexHtml).toContain('<link rel="icon" href="/appicon.png"')
  })
})
