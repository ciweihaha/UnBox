// playbackStall.ts — 播放停滞检测：进度几秒不动就判死。
//
// 直播冻住时播放器不报错、也不发 buffering：video 的 currentTime 停住，
// timeupdate 不再推进，界面却还显示「正在播放」。PlaybackHealthMonitor
// 只看 playing/buffering/error 信号，这种静默冻住它看不到。
// 本模块只盯进度：播起来后进度超过阈值不动就回调一次；暂停、跳转、切集
// 时停表，不误判。纯 TypeScript，可单测；PlaybackView 负责喂进度，
// App.vue 负责收到回调后重连/换线。

/** 直播进度停滞多久判死（毫秒）：SCF 中继取分片一般 1–3 秒，10 秒不动就是死了。 */
export const LIVE_STALL_MS = 10_000
/** 点播进度停滞多久判死（毫秒）：点播分片更大，耐心稍多一点。 */
export const VOD_STALL_MS = 15_000

/** 停滞检测的节流：多久检查一次进度时钟。 */
export const STALL_CHECK_MS = 2_000

export class PlaybackStallDetector {
  private lastAdvanceAt = 0
  private lastTime = -1
  private timer: ReturnType<typeof setInterval> | null = null
  private stalled = false
  private watching = false
  private playing = false

  constructor(
    private readonly thresholdMs: number,
    private readonly onStall: () => void,
    private readonly now: () => number = () => Date.now(),
  ) {}

  /** start 开始盯：播起来（首个 playing / 首个进度）后调用。 */
  start(): void {
    this.watching = true
    this.playing = true
    this.stalled = false
    this.lastAdvanceAt = this.now()
    this.lastTime = -1
    if (this.timer === null) {
      this.timer = setInterval(() => this.check(), STALL_CHECK_MS)
    }
  }

  /** setPlaying 暂停时停表、恢复时重计时：暂停不是冻住。 */
  setPlaying(playing: boolean): void {
    if (!this.watching || this.stalled) return
    this.playing = playing
    if (playing) this.lastAdvanceAt = this.now()
  }

  /** progress 喂进度（秒）：前进就刷新时钟；倒退/不动不刷新。 */
  progress(time: number): void {
    if (!this.watching || this.stalled || !this.playing) return
    if (time > this.lastTime) {
      this.lastTime = time
      this.lastAdvanceAt = this.now()
    }
  }

  /** stop 停表：切集、切台、卸载时调用，之后不再回调。 */
  stop(): void {
    this.watching = false
    this.playing = false
    this.stalled = false
    if (this.timer !== null) {
      clearInterval(this.timer)
      this.timer = null
    }
  }

  private check(): void {
    if (!this.watching || this.stalled || !this.playing) return
    if (this.now() - this.lastAdvanceAt >= this.thresholdMs) {
      this.stalled = true
      this.onStall()
    }
  }
}
