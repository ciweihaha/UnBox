// Package shell 的直播实时字幕胶水：把 internal/livesub 的 Manager
// 暴露给前端，并把开关/模型/窗口持久化到 store.kv（与翻译设置同一风格）。
//
// 翻译复用现有的 Ollama 设置（模型/地址/开关），不另起一套；
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
	"github.com/unbox/unbox/internal/translate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

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
// 首次使用会下载程序与模型并缓存）。已在跑时直接成功；
// 已有一次启动在进行时直接返回（等那次完成）。
func (s *ShellService) StartLiveSubtitles() error {
	s.liveSubMu.Lock()
	if s.liveSub != nil && s.liveSub.Running() {
		s.liveSubMu.Unlock()
		return nil
	}
	if s.liveSubStarting {
		s.liveSubMu.Unlock()
		return nil
	}
	s.liveSubStarting = true
	gen := s.liveSubGen
	root, _ := os.UserConfigDir()
	dataDir := filepath.Join(root, "unbox", "livesub")
	tr := s.GetTranslateSettings()
	mgr := s.newLiveSubManager(livesub.ManagerConfig{
		WindowSeconds: s.GetLiveSubtitleSettings().WindowSeconds,
		WhisperModel:  s.GetLiveSubtitleSettings().WhisperModel,
		DataDir:       dataDir,
		Translator:    &ollamaTranslator{client: translateClientFor(tr.Endpoint, tr.Model)},
		Emit:          s.emitLiveSubtitle,
	})
	s.liveSub = mgr
	s.liveSubMu.Unlock()

	err := mgr.Start(context.Background())
	s.liveSubMu.Lock()
	s.liveSubStarting = false
	if err != nil {
		s.liveSub = nil
		s.liveSubMu.Unlock()
		return err
	}
	// 启动期间有人点了停止：刚拉起的管线立即停掉，别让它空转。
	if s.liveSubGen != gen {
		s.liveSubMu.Unlock()
		_ = mgr.Stop()
		s.liveSubMu.Lock()
		if s.liveSub == mgr {
			s.liveSub = nil
		}
	}
	s.liveSubMu.Unlock()
	return nil
}

// StopLiveSubtitles 停掉直播实时字幕管线与 whisper-server；未启动时直接成功。
func (s *ShellService) StopLiveSubtitles() error {
	s.liveSubMu.Lock()
	mgr := s.liveSub
	s.liveSub = nil
	s.liveSubGen++
	s.liveSubMu.Unlock()
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
	s.liveSubMu.Unlock()
	if mgr == nil {
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
func (s *ShellService) ensureLiveSubtitlesForChannel(backend string) {
	if backend != "web" {
		return
	}
	if !s.GetLiveSubtitleSettings().Enabled {
		return
	}
	s.liveSubMu.Lock()
	mgr := s.liveSub
	s.liveSubMu.Unlock()
	if mgr != nil && mgr.Running() {
		mgr.Reset()
		return
	}
	go func() {
		if err := s.StartLiveSubtitles(); err != nil {
			log.Printf("启动直播实时字幕失败: %v", err)
		}
	}()
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

// ollamaTranslator 把 translate.Client 适配成 livesub.Translator。
type ollamaTranslator struct {
	client *translate.Client
}

func (o *ollamaTranslator) Translate(ctx context.Context, text string) (string, error) {
	return o.client.Translate(ctx, text)
}
