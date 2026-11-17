// liveAudio.test.ts — 直播音频抓取：下采样、base64、混声道与抓取启停。
import { describe, expect, it, vi } from 'vitest'
import {
  LIVE_AUDIO_SAMPLE_RATE,
  LiveAudioCapture,
  base64FromInt16,
  downsampleTo16k,
  mixToMono,
  type LiveAudioPusher,
} from './liveAudio'

describe('downsampleTo16k', () => {
  it('48kHz 按 3:1 抽成 16kHz', () => {
    const input = new Float32Array(4800)
    for (let i = 0; i < input.length; i++) input[i] = i / 4800
    const out = downsampleTo16k(input, 48000)
    expect(out.length).toBe(1600)
    // 首个采样接近 0，末个接近 1。
    expect(out[0]).toBe(0)
    expect(out[out.length - 1]).toBeGreaterThan(32000)
  })

  it('44.1kHz 线性插值到 16kHz', () => {
    const input = new Float32Array(4410)
    input.fill(0.5)
    const out = downsampleTo16k(input, 44100)
    expect(out.length).toBe(1600)
    expect(out.every((v) => v === 16384)).toBe(true)
  })

  it('已是 16kHz 时直接转整型', () => {
    const out = downsampleTo16k(new Float32Array([0, 0.5, -0.5, 1, -1]), 16000)
    expect(Array.from(out)).toEqual([0, 16384, -16384, 32767, -32768])
  })

  it('空输入返回空数组', () => {
    expect(downsampleTo16k(new Float32Array(0), 48000).length).toBe(0)
  })

  it('超幅钳制到 [-1, 1]', () => {
    const out = downsampleTo16k(new Float32Array([2, -2]), 16000)
    expect(Array.from(out)).toEqual([32767, -32768])
  })
})

describe('base64FromInt16', () => {
  it('往返解码后字节一致（小端）', () => {
    const samples = new Int16Array([0, 1000, -1000, 32767, -32768])
    const b64 = base64FromInt16(samples)
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
    expect(bytes.length).toBe(10)
    const back = new Int16Array(bytes.buffer)
    expect(Array.from(back)).toEqual(Array.from(samples))
  })

  it('空采样编成空串', () => {
    expect(base64FromInt16(new Int16Array(0))).toBe('')
  })
})

describe('mixToMono', () => {
  it('立体声取平均', () => {
    const out = mixToMono([new Float32Array([1, 0]), new Float32Array([0, 1])])
    expect(Array.from(out)).toEqual([0.5, 0.5])
  })

  it('单声道原样返回', () => {
    const ch = new Float32Array([0.25, -0.25])
    expect(mixToMono([ch])).toBe(ch)
  })

  it('无声道返回空数组', () => {
    expect(mixToMono([]).length).toBe(0)
  })
})

describe('LiveAudioCapture', () => {
  function mockAudioContext() {
    const destination = {}
    const source = { connect: vi.fn(), disconnect: vi.fn() }
    const ctx = {
      state: 'suspended' as string,
      sampleRate: 48000,
      destination,
      resume: vi.fn(async () => {}),
      close: vi.fn(async () => {}),
      createMediaElementSource: vi.fn(() => source),
      audioWorklet: { addModule: vi.fn(async () => {}) },
    }
    return { ctx: ctx as unknown as AudioContext, source }
  }

  function mockPusher(): LiveAudioPusher & { pushed: string[] } {
    const pushed: string[] = []
    return {
      pushed,
      pushLiveAudioChunk: vi.fn(async (b64: string) => {
        pushed.push(b64)
      }),
    }
  }

  it('start 建源节点并连输出、挂 worklet 分流', async () => {
    const { ctx, source } = mockAudioContext()
    const pusher = mockPusher()
    const video = document.createElement('video')
    const tapPort = { onmessage: null as ((ev: MessageEvent<ArrayBuffer>) => void) | null }
    const tap = { port: tapPort, disconnect: vi.fn() }
    const cap = new LiveAudioCapture(video, pusher, () => {}, {
      createAudioContext: () => ctx,
      createWorkletNode: async () => tap as unknown as AudioWorkletNode,
    })
    await cap.start()
    expect(ctx.createMediaElementSource).toHaveBeenCalledWith(video)
    // 源节点直连输出，保证正常出声。
    expect(source.connect).toHaveBeenCalledWith(ctx.destination)
    expect(ctx.audioWorklet.addModule).toHaveBeenCalled()
    expect(source.connect).toHaveBeenCalledWith(tap)
    expect(cap.running).toBe(true)
    // worklet 推来一批音频，应 base64 推给后端。
    const pcm = new Int16Array([100, -100, 200]).buffer
    tapPort.onmessage?.({ data: pcm } as MessageEvent<ArrayBuffer>)
    await vi.waitFor(() => expect(pusher.pushed.length).toBe(1))
    const bytes = Uint8Array.from(atob(pusher.pushed[0] as string), (c) => c.charCodeAt(0))
    expect(new Int16Array(bytes.buffer)).toEqual(new Int16Array([100, -100, 200]))
  })

  it('重复 start 不重复建源节点', async () => {
    const { ctx } = mockAudioContext()
    const pusher = mockPusher()
    const cap = new LiveAudioCapture(document.createElement('video'), pusher, () => {}, {
      createAudioContext: () => ctx,
      createWorkletNode: async () => ({ port: {}, disconnect: vi.fn() }) as unknown as AudioWorkletNode,
    })
    await cap.start()
    await cap.start()
    expect(ctx.createMediaElementSource).toHaveBeenCalledTimes(1)
  })

  it('stop 断开分流、running 变 false，可再次 start', async () => {
    const { ctx, source } = mockAudioContext()
    const pusher = mockPusher()
    const cap = new LiveAudioCapture(document.createElement('video'), pusher, () => {}, {
      createAudioContext: () => ctx,
      createWorkletNode: async () => ({ port: {}, disconnect: vi.fn() }) as unknown as AudioWorkletNode,
    })
    await cap.start()
    cap.stop()
    expect(cap.running).toBe(false)
    expect(source.disconnect).toHaveBeenCalled()
    await cap.start()
    expect(cap.running).toBe(true)
    // 第二次 start 不应再调 createMediaElementSource（同一 video 只能调一次）。
    expect(ctx.createMediaElementSource).toHaveBeenCalledTimes(1)
  })

  it('AudioContext 建失败时调 onError、不抛错', async () => {
    const onError = vi.fn()
    const cap = new LiveAudioCapture(document.createElement('video'), mockPusher(), onError, {
      createAudioContext: () => {
        throw new Error('no audio')
      },
    })
    await cap.start()
    expect(onError).toHaveBeenCalledWith(expect.stringContaining('直播音频抓取失败'))
    expect(cap.running).toBe(false)
  })

  it('采样率常量与后端一致', () => {
    expect(LIVE_AUDIO_SAMPLE_RATE).toBe(16000)
  })
})
