// Package shell 的直播实时字幕胶水：把 internal/livesub 的 Manager
// 暴露给前端，并把开关/模型/窗口持久化到 store.kv（与翻译设置同一风格）。
//
// 翻译复用现有的字幕翻译设置（免费接口服务商/Key/开关），不另起一套；
// 语音识别（Whisper）模型与窗口时长是直播字幕自己的设置。
// 音频来自前端 Web Audio（抓 <video> 的声音，与画面同步），
// 后端只负责识别、翻译、推送；因此只在 Web 播放时启动，
// mpv 后端没有 <video> 元素，抓不到音频。
package shell

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/livesub"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// liveSubRetryDelay 是 ensure 后台启动失败后的重试间隔（测试可调小）。
var liveSubRetryDelay = 2 * time.Second

// 直播实时字幕设置在 store.kv 里的键。
const (
	liveSubEnabledKey = "livesub.enabled"
	liveSubModelKey   = "livesub.model"
	liveSubWindowKey  = "livesub.window"
)

// LiveSubtitleSettings 是直播实时字幕选项：默认关闭，
// 识别模型默认 tiny.en（快），窗口默认 1.5 秒（延迟与质量的平衡点）。
type LiveSubtitleSettings struct {
	Enabled       bool
	WhisperModel  string
	WindowSeconds float64
}

// LiveSubtitleEvent 是一条推给前端的直播字幕（Wails 事件 live-subtitle）。
type LiveSubtitleEvent struct {
	Text      string
	LatencyMs int64
}

// LiveSubtitleStatus 是直播实时字幕的运行状态（供设置页回显与排障）。
type LiveSubtitleStatus struct {
	Running          bool
	Enabled          bool
	WhisperModel     string
	WindowSeconds    float64
	WindowsProcessed int
	WindowsDropped   int
	LastLatencyMs    int64
	AvgLatencyMs     int64
	LastSubtitle     string
	Error            string
}

// GetLiveSubtitleSettings 返回直播实时字幕设置；未设置或非法时回退默认值。
func (s *ShellService) GetLiveSubtitleSettings() LiveSubtitleSettings {
	settings := LiveSubtitleSettings{
		Enabled:       false,
		WhisperModel:  "tiny.en",
		WindowSeconds: livesub.DefaultWindowSeconds,
	}
	if s.store == nil {
		return settings
	}
	settings.Enabled = s.getPlaybackSetting(liveSubEnabledKey)
	if v, ok, err := s.store.GetKV(liveSubModelKey); err == nil && ok && strings.TrimSpace(v) != "" {
		settings.WhisperModel = strings.TrimSpace(v)
	}
	if v, ok, err := s.store.GetKV(liveSubWindowKey); err == nil && ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 && f <= 10 {
			settings.WindowSeconds = f
		}
	}
	return settings
}

// SetLiveSubtitleSettings 持久化直播实时字幕设置。
func (s *ShellService) SetLiveSubtitleSettings(settings LiveSubtitleSettings) error {
	if s.store == nil {
		return nil
	}
	window := strconv.FormatFloat(settings.WindowSeconds, 'f', -1, 64)
	for _, kv := range []struct {
		key   string
		value string
	}{
		{liveSubEnabledKey, strconv.FormatBool(settings.Enabled)},
		{liveSubModelKey, strings.TrimSpace(settings.WhisperModel)},
		{liveSubWindowKey, window},
	} {
		if err := s.store.SetKV(kv.key, kv.value); err != nil {
			return err
		}
	}
	return nil
}

// StartLiveSubtitles 启动直播实时字幕管线（拉起 whisper-server，
// 首次使用会下载程序与模型并缓存）。已在跑时直接成功。
//
// 首次播放常遇到的坑：一次启动可能卡在下载（单个文件最长 30 分钟），
// 期间切台/过期请求会停掉它，随后新的播放请求又要启动——
// 旧代码里后来者见 liveSubStarting 就静默返回 nil，先那次启动结束
// 又因代次变化被停掉，结果管线再也没起来、首次播放全程没字幕。
// 所以后来者不再静默丢弃：等进行中的启动结束，若期间没人停过，
// 就自己成为新的启动者；若等的时候被停了，直接返回（别复活管线）。
func (s *ShellService) StartLiveSubtitles() error {
	for {
		s.liveSubMu.Lock()
		if s.liveSub != nil && s.liveSub.Running() {
			s.liveSubMu.Unlock()
			return nil
		}
		if done := s.liveSubStartDone; done != nil {
			gen := s.liveSubGen
			s.liveSubMu.Unlock()
			<-done
			// 等待期间有人停过（代次变了）：这次启动已作废，别复活管线。
			s.liveSubMu.Lock()
			stopped := s.liveSubGen != gen
			s.liveSubMu.Unlock()
			if stopped {
				return nil
			}
			continue
		}
		gen := s.liveSubGen
		ctx, cancel := context.WithCancel(context.Background())
		s.liveSubStarting = true
		s.liveSubStartDone = make(chan struct{})
		s.liveSubStartCancel = cancel
		s.liveSubMu.Unlock()

		root, _ := os.UserConfigDir()
		dataDir := filepath.Join(root, "unbox", "livesub")
		mgr := s.newLiveSubManager(livesub.ManagerConfig{
			WindowSeconds: s.GetLiveSubtitleSettings().WindowSeconds,
			WhisperModel:  s.GetLiveSubtitleSettings().WhisperModel,
			DataDir:       dataDir,
			Translator:    s.translatorFor(s.GetTranslateSettings()),
			Emit:          s.emitLiveSubtitle,
		})
		s.liveSubMu.Lock()
		s.liveSub = mgr
		s.liveSubMu.Unlock()

		err := mgr.Start(ctx)
		s.liveSubMu.Lock()
		s.liveSubStarting = false
		close(s.liveSubStartDone)
		s.liveSubStartDone = nil
		s.liveSubStartCancel = nil
		if err != nil {
			s.liveSub = nil
			if errors.Is(err, context.Canceled) {
				// 停播/切台打断了这次启动：不是故障，别记错误、别重试。
				s.liveSubMu.Unlock()
				return nil
			}
			s.liveSubLastErr = err.Error()
			s.liveSubMu.Unlock()
			return err
		}
		s.liveSubLastErr = ""
		// 启动期间有人点了停止：刚拉起的管线立即停掉，别让它空转。
		if s.liveSubGen != gen {
			s.liveSubMu.Unlock()
			_ = mgr.Stop()
			s.liveSubMu.Lock()
			if s.liveSub == mgr {
				s.liveSub = nil
			}
			s.liveSubMu.Unlock()
			return nil
		}
		s.liveSubMu.Unlock()
		return nil
	}
}

// StopLiveSubtitles 停掉直播实时字幕管线与 whisper-server；未启动时直接成功。
// 进行中的启动也会被取消（下载立即中断），不让它占着启动守卫、
// 拖垮随后新的播放请求的启动。
func (s *ShellService) StopLiveSubtitles() error {
	s.liveSubMu.Lock()
	mgr := s.liveSub
	s.liveSub = nil
	s.liveSubGen++
	cancel := s.liveSubStartCancel
	s.liveSubStartCancel = nil
	s.liveSubLastErr = ""
	s.liveSubMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if mgr == nil {
		return nil
	}
	return mgr.Stop()
}

// PushLiveAudioChunk 接收前端 Web Audio 抓到的 PCM（base64 编码的
// 16kHz 单声道 16 位小端采样），推给字幕管线。
// 管线没跑时直接丢弃；base64 非法返回错误。
func (s *ShellService) PushLiveAudioChunk(b64 string) error {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("音频数据不是合法 base64: %w", err)
	}
	if len(raw) == 0 || len(raw)%2 != 0 {
		return errors.New("音频数据长度不对（应为 16 位采样）")
	}
	samples := make([]int16, len(raw)/2)
	for i := range samples {
		samples[i] = int16(raw[i*2]) | int16(raw[i*2+1])<<8
	}
	s.liveSubMu.Lock()
	mgr := s.liveSub
	s.liveSubMu.Unlock()
	if mgr == nil {
		return nil
	}
	mgr.PushAudio(samples)
	return nil
}

// LiveSubtitleStatus 返回直播实时字幕的运行状态。
func (s *ShellService) LiveSubtitleStatus() LiveSubtitleStatus {
	settings := s.GetLiveSubtitleSettings()
	st := LiveSubtitleStatus{
		Enabled:       settings.Enabled,
		WhisperModel:  settings.WhisperModel,
		WindowSeconds: settings.WindowSeconds,
	}
	s.liveSubMu.Lock()
	mgr := s.liveSub
	lastErr := s.liveSubLastErr
	s.liveSubMu.Unlock()
	if mgr == nil {
		// 管线未建时也回显最近一次启动失败：否则首次启动下载失败后，
		// 设置页只显示“未启动”、错误原因无处可查。
		st.Error = lastErr
		return st
	}
	ms := mgr.Stats()
	st.Running = ms.Running
	st.WindowsProcessed = ms.WindowsProcessed
	st.WindowsDropped = ms.WindowsDropped
	st.LastLatencyMs = ms.LastLatency.Milliseconds()
	st.AvgLatencyMs = ms.AvgLatency.Milliseconds()
	st.LastSubtitle = ms.LastSubtitle
	st.Error = ms.Error
	return st
}

// newLiveSubManager 建 Manager；测试可注入 liveSubFactory 换假实现。
func (s *ShellService) newLiveSubManager(cfg livesub.ManagerConfig) *livesub.Manager {
	if s.liveSubFactory != nil {
		return s.liveSubFactory(cfg)
	}
	return livesub.NewManager(cfg)
}

// ensureLiveSubtitlesForChannel 在直播准备成功后（Web 后端）
// 确保字幕管线在跑：已跑就清掉旧台残音，没跑就后台启动
// （首次下载要几分钟，不能堵住播放准备；字幕下载好就开始出）。
// 开关关闭或非 Web 后端时什么都不做；启动失败只记日志，不挡播放。
// 后台启动带有限重试：首次下载偶发失败时不用用户手动重切台；
// 停播/切台后重试立即停，不复活管线。
func (s *ShellService) ensureLiveSubtitlesForChannel(backend string) {
	if backend != "web" {
		return
	}
	if !s.GetLiveSubtitleSettings().Enabled {
		return
	}
	s.liveSubMu.Lock()
	mgr := s.liveSub
	gen := s.liveSubGen
	s.liveSubMu.Unlock()
	if mgr != nil && mgr.Running() {
		mgr.Reset()
		return
	}
	go s.startLiveSubtitlesWithRetry(gen)
}

// startLiveSubtitlesWithRetry 后台启动字幕管线，最多 3 次；
// 每次重试前确认没人停过（代次没变），否则直接退出。
func (s *ShellService) startLiveSubtitlesWithRetry(gen uint64) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if s.liveSubGenNow() != gen {
				return
			}
			time.Sleep(liveSubRetryDelay)
			if s.liveSubGenNow() != gen {
				return
			}
			if st := s.LiveSubtitleStatus(); st.Running {
				return
			}
		}
		if err := s.StartLiveSubtitles(); err != nil {
			log.Printf("启动直播实时字幕失败: %v", err)
			continue
		}
		return
	}
}

// liveSubGenNow 读当前停止代次。
func (s *ShellService) liveSubGenNow() uint64 {
	s.liveSubMu.Lock()
	defer s.liveSubMu.Unlock()
	return s.liveSubGen
}

// stopLiveSubtitlesQuietly 停字幕管线（切台/停播时调用），失败只记日志。
func (s *ShellService) stopLiveSubtitlesQuietly() {
	if err := s.StopLiveSubtitles(); err != nil {
		log.Printf("停止直播实时字幕失败: %v", err)
	}
}

// emitLiveSubtitle 经 Wails 事件 live-subtitle 把字幕推给前端；
// application 未就绪时静默丢弃（与 emitPlaybackEvent 同一风格）。
func (s *ShellService) emitLiveSubtitle(text string, latency time.Duration) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("live-subtitle", LiveSubtitleEvent{Text: text, LatencyMs: latency.Milliseconds()})
}
