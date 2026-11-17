// liveAudio.ts — 直播音频抓取：从 <video> 经 Web Audio 抓声音，
// 下采样到 16kHz 单声道 16 位 PCM，分批推给后端做识别。
//
// 与画面同一路音频，字幕与说话天然同步。抓到的声音经
// ShellService.PushLiveAudioChunk（base64）推给后端。
// 注意：视频静音时抓到的是静音，识别不到文字、也就不出字幕。
//
// AudioWorklet 跑在音频线程里做下采样；主线程只负责 base64 与推送。

/** 目标采样率：Whisper 只要 16kHz 单声道 16 位 PCM。 */
export const LIVE_AUDIO_SAMPLE_RATE = 16000

/** 每批推送的音频时长（秒）：0.5 秒一批，后端攒满一窗（默认 2 秒）再识别。 */
export const LIVE_AUDIO_CHUNK_SECONDS = 0.5

/** 后端音频入口的最小形状（对应 ShellService.PushLiveAudioChunk）。 */
export interface LiveAudioPusher {
  pushLiveAudioChunk(base64: string): Promise<void>
}

/**
 * downsampleTo16k 把任意采样率的 float32 单声道采样线性插值重采样到 16kHz，
 * 再转成 16 位整型。Web Audio 常见 48kHz/44.1kHz，线性插值对语音识别足够。
 */
export function downsampleTo16k(input: Float32Array, inputRate: number): Int16Array {
  if (input.length === 0 || inputRate <= 0) return new Int16Array(0)
  if (inputRate === LIVE_AUDIO_SAMPLE_RATE) {
    const out = new Int16Array(input.length)
    for (let i = 0; i < input.length; i++) out[i] = floatToInt16(input[i] ?? 0)
    return out
  }
  const ratio = inputRate / LIVE_AUDIO_SAMPLE_RATE
  const outLen = Math.floor(input.length / ratio)
  const out = new Int16Array(outLen)
  for (let i = 0; i < outLen; i++) {
    const pos = i * ratio
    const idx = Math.floor(pos)
    const frac = pos - idx
    const a = input[idx] ?? 0
    const b = input[Math.min(idx + 1, input.length - 1)] ?? 0
    out[i] = floatToInt16(a + (b - a) * frac)
  }
  return out
}

function floatToInt16(v: number): number {
  const clamped = Math.max(-1, Math.min(1, v))
  return clamped < 0 ? Math.round(clamped * 32768) : Math.round(clamped * 32767)
}

/** base64FromInt16 把 16 位采样编成 base64（小端字节序），供后端解码。 */
export function base64FromInt16(samples: Int16Array): string {
  const bytes = new Uint8Array(samples.buffer, samples.byteOffset, samples.byteLength)
  let binary = ''
  const CHUNK = 8192
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(binary)
}

/**
 * mixToMono 把多声道 float32 采样混成单声道（左右声道取平均）；
 * 单声道直接原样返回。
 */
export function mixToMono(channels: Float32Array[]): Float32Array {
  if (channels.length === 0) return new Float32Array(0)
  if (channels.length === 1) return channels[0] as Float32Array
  const len = channels[0]?.length ?? 0
  const out = new Float32Array(len)
  for (let i = 0; i < len; i++) {
    let sum = 0
    for (const ch of channels) sum += ch[i] ?? 0
    out[i] = sum / channels.length
  }
  return out
}

// workletCode 是下采样节点的代码（Blob URL 载入）：
// 输入是播放器的声音（上下文采样率、可能立体声），
// 每攒够 0.5 秒就下采样到 16kHz 单声道 int16，发给主线程。
const workletCode = `
class LiveAudioTap extends AudioWorkletProcessor {
  constructor() {
    super()
    this.buf = []
    this.target = ${LIVE_AUDIO_SAMPLE_RATE}
    this.chunk = Math.floor(this.target * ${LIVE_AUDIO_CHUNK_SECONDS})
  }
  process(inputs) {
    const input = inputs[0]
    if (!input || input.length === 0) return true
    const len = input[0].length
    for (let i = 0; i < len; i++) {
      let sum = 0
      for (let c = 0; c < input.length; c++) sum += input[c][i] || 0
      this.buf.push(sum / input.length)
    }
    // 按上下文采样率换算：攒够 0.5 秒的量再下采样推送。
    const need = Math.floor(sampleRate * ${LIVE_AUDIO_CHUNK_SECONDS})
    while (this.buf.length >= need) {
      const take = this.buf.splice(0, need)
      const ratio = sampleRate / this.target
      const outLen = Math.floor(take.length / ratio)
      const out = new Int16Array(outLen)
      for (let i = 0; i < outLen; i++) {
        const pos = i * ratio
        const idx = Math.floor(pos)
        const frac = pos - idx
        const a = take[idx] || 0
        const b = take[Math.min(idx + 1, take.length - 1)] || 0
        let v = a + (b - a) * frac
        v = Math.max(-1, Math.min(1, v))
        out[i] = v < 0 ? Math.round(v * 32768) : Math.round(v * 32767)
      }
      this.port.postMessage(out.buffer, [out.buffer])
    }
    return true
  }
}
registerProcessor('live-audio-tap', LiveAudioTap)
`

export interface LiveAudioCaptureOptions {
  /** 建 AudioContext（测试可注入假实现）。 */
  createAudioContext?: () => AudioContext
  /** 往 worklet 挂载处理节点（测试可注入假实现）。 */
  createWorkletNode?: (ctx: AudioContext, name: string, url: string) => Promise<AudioWorkletNode>
}

/**
 * LiveAudioCapture 从 <video> 抓音频推给后端。
 * 同一个 <video> 只能调一次 createMediaElementSource，
 * 因此源节点只建一次、一直连着输出（保证正常出声），
 * 开关字幕只接/断 worklet 这一路分流。
 */
export class LiveAudioCapture {
  private video: HTMLVideoElement
  private pusher: LiveAudioPusher
  private onError: (msg: string) => void
  private opts: LiveAudioCaptureOptions
  private ctx: AudioContext | null = null
  private source: MediaElementAudioSourceNode | null = null
  private tap: AudioWorkletNode | null = null
  private workletUrl: string | null = null
  private runningFlag = false

  constructor(
    video: HTMLVideoElement,
    pusher: LiveAudioPusher,
    onError: (msg: string) => void = () => {},
    opts: LiveAudioCaptureOptions = {},
  ) {
    this.video = video
    this.pusher = pusher
    this.onError = onError
    this.opts = opts
  }

  get running(): boolean {
    return this.runningFlag
  }

  /** start 开始抓音频并推送；重复调用直接成功。 */
  async start(): Promise<void> {
    if (this.runningFlag) return
    try {
      if (!this.ctx) {
        const createCtx =
          this.opts.createAudioContext ??
          (() => new (window.AudioContext ??
            (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext)())
        this.ctx = createCtx()
        const createSource = (this.ctx as AudioContext).createMediaElementSource(this.video)
        this.source = createSource
        // 源节点直连输出：不断开，正常出声不受字幕开关影响。
        this.source.connect(this.ctx.destination)
      }
      if (this.ctx.state === 'suspended') await this.ctx.resume()
      if (!this.workletUrl) {
        const blob = new Blob([workletCode], { type: 'application/javascript' })
        this.workletUrl = URL.createObjectURL(blob)
        await this.ctx.audioWorklet.addModule(this.workletUrl)
      }
      const ctx = this.ctx
      const createNode =
        this.opts.createWorkletNode ??
        (async (c: AudioContext, name: string) => new AudioWorkletNode(c, name))
      this.tap = await createNode(ctx, 'live-audio-tap', this.workletUrl)
      this.tap.port.onmessage = (ev: MessageEvent<ArrayBuffer>) => {
        void this.pushChunk(new Int16Array(ev.data))
      }
      this.source?.connect(this.tap)
      this.runningFlag = true
    } catch (e) {
      this.onError(`直播音频抓取失败：${e instanceof Error ? e.message : String(e)}`)
    }
  }

  /** stop 断开抓取（不断源节点，正常出声继续）；可再次 start。 */
  stop(): void {
    if (!this.runningFlag) return
    this.runningFlag = false
    try {
      if (this.tap && this.source) this.source.disconnect(this.tap)
    } catch {
      // 断开失败不致命：节点已随上下文释放或重复断开。
    }
    if (this.tap) {
      try {
        this.tap.disconnect()
      } catch {
        // 同上。
      }
      this.tap = null
    }
  }

  /** destroy 彻底释放：停抓取并关 AudioContext（组件卸载时调用）。 */
  destroy(): void {
    this.stop()
    if (this.workletUrl) {
      URL.revokeObjectURL(this.workletUrl)
      this.workletUrl = null
    }
    if (this.ctx) {
      void this.ctx.close().catch(() => {})
      this.ctx = null
      this.source = null
    }
  }

  private async pushChunk(samples: Int16Array): Promise<void> {
    if (!this.runningFlag || samples.length === 0) return
    try {
      await this.pusher.pushLiveAudioChunk(base64FromInt16(samples))
    } catch {
      // 推送失败（后端没跑/网络抖动）不抛到音频线程，只丢掉这一批。
    }
  }
}
