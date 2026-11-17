package livesub

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Transcriber 语音识别接口：WAV 音频 -> 英文文本。
type Transcriber interface {
	Transcribe(ctx context.Context, wav []byte) (string, error)
}

// Translator 翻译接口：英文 -> 中文。
type Translator interface {
	Translate(ctx context.Context, text string) (string, error)
}

// WhisperServer 管理一个 whisper-server 子进程。
type WhisperServer interface {
	// Start 拉起服务，返回 HTTP 根地址（如 http://127.0.0.1:17880）。
	Start(ctx context.Context, binPath, modelPath string) (endpoint string, err error)
	// Stop 停掉服务。
	Stop() error
}

// emittedSubtitle 是一条推给前端的字幕。
type emittedSubtitle struct {
	Text    string
	Latency time.Duration
}

// Stats 是直播实时字幕的运行统计（供设置页与汇报用）。
type Stats struct {
	Running          bool
	WindowsProcessed int
	WindowsDropped   int
	LastLatency      time.Duration // 最近一窗的端到端延迟（窗口+识别+翻译）
	AvgLatency       time.Duration // 平均端到端延迟
	LastASR          time.Duration // 最近一窗识别耗时
	LastTranslate    time.Duration // 最近一窗翻译耗时
	LastText         string        // 最近一条原文
	LastSubtitle     string        // 最近一条字幕
	Error            string        // 最近一次错误（空表示正常）
}

// ManagerConfig 是 Manager 的配置；依赖可注入假实现以便测试。
type ManagerConfig struct {
	WindowSeconds float64 // 窗口秒数，<=0 用默认值
	WhisperModel  string  // tiny.en / base.en，空用 tiny.en
	DataDir       string  // 资源缓存根目录（程序与模型放其下 livesub/）
	Transcriber   Transcriber
	Translator    Translator // 为空时不翻译，直接出英文
	Server        WhisperServer
	EnsureAssets  func(ctx context.Context, dir, model string, p ProgressFunc) (binPath, modelPath string, err error)
	Emit          func(text string, latency time.Duration)
}

// Manager 串行处理直播音频窗口：识别 -> 翻译 -> 推送。
// 串行保证字幕顺序与画面一致；掉队时丢旧窗追直播，不越攒越落后。
type Manager struct {
	cfg     ManagerConfig
	buf     *WindowBuffer
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	server  WhisperServer
	trans   Transcriber

	statsMu       sync.Mutex
	processed     int
	dropped       int
	latencySum    time.Duration
	lastLatency   time.Duration
	lastASR       time.Duration
	lastTranslate time.Duration
	lastText      string
	lastSubtitle  string
	lastErr       string
	lastSource    string
}

// NewManager 构造 Manager；不启动任何进程，Start 才拉起 whisper-server。
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = DefaultWindowSeconds
	}
	if cfg.WhisperModel == "" {
		cfg.WhisperModel = "tiny.en"
	}
	return &Manager{
		cfg:  cfg,
		buf:  NewWindowBuffer(cfg.WindowSeconds),
		done: make(chan struct{}),
	}
}

// Running 返回字幕管线是否在跑。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Start 拉起 whisper-server 并开始处理音频窗口；重复启动直接成功。
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	ensure := m.cfg.EnsureAssets
	if ensure == nil {
		ensure = EnsureAssets
	}
	dataDir := m.cfg.DataDir
	if dataDir == "" {
		return fmt.Errorf("未配置资源缓存目录")
	}
	binPath, modelPath, err := ensure(ctx, dataDir, m.cfg.WhisperModel, nil)
	if err != nil {
		m.setError(err.Error())
		return err
	}
	server := m.cfg.Server
	if server == nil {
		server = &execWhisperServer{}
	}
	endpoint, err := server.Start(ctx, binPath, modelPath)
	if err != nil {
		m.setError(err.Error())
		return err
	}
	trans := m.cfg.Transcriber
	if trans == nil {
		trans = NewWhisperClient(endpoint, 30*time.Second)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.running = true
	m.cancel = cancel
	m.server = server
	m.trans = trans
	m.mu.Unlock()
	go m.loop(runCtx)
	return nil
}

// Stop 停掉管线与 whisper-server；未启动时直接成功。
func (m *Manager) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	cancel := m.cancel
	server := m.server
	m.running = false
	m.cancel = nil
	m.server = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.buf.Clear()
	if server != nil {
		return server.Stop()
	}
	return nil
}

// PushAudio 推一段 16kHz 单声道 PCM；未启动时直接丢弃。
func (m *Manager) PushAudio(samples []int16) {
	if !m.Running() {
		return
	}
	m.buf.Push(samples)
}

// Reset 清空音频缓冲与去重状态（切台时调用）：
// 不停 whisper-server，只把旧台的残音与上一条文本清掉，
// 新台的字幕从干净状态开始。
func (m *Manager) Reset() {
	m.buf.Clear()
	m.statsMu.Lock()
	m.lastSource = ""
	m.lastText = ""
	m.lastSubtitle = ""
	m.statsMu.Unlock()
}

// Stats 返回运行统计（快照）。
func (m *Manager) Stats() Stats {
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	avg := time.Duration(0)
	if m.processed > 0 {
		avg = m.latencySum / time.Duration(m.processed)
	}
	return Stats{
		Running:          m.Running(),
		WindowsProcessed: m.processed,
		WindowsDropped:   m.dropped,
		LastLatency:      m.lastLatency,
		AvgLatency:       avg,
		LastASR:          m.lastASR,
		LastTranslate:    m.lastTranslate,
		LastText:         m.lastText,
		LastSubtitle:     m.lastSubtitle,
		Error:            m.lastErr,
	}
}

func (m *Manager) setError(msg string) {
	m.statsMu.Lock()
	m.lastErr = msg
	m.statsMu.Unlock()
}

// loop 每 50ms 看一次有没有攒满的窗口，有就串行处理。
func (m *Manager) loop(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.processOne(ctx)
		}
	}
}

// processOne 处理一个窗口：识别 -> 去噪去重 -> 翻译 -> 推送。
func (m *Manager) processOne(ctx context.Context) {
	// 掉队追赶：只留最新一窗，旧窗直接丢（记数），不让字幕越落越远。
	for m.buf.PendingWindows() > 1 {
		m.buf.DropOldest()
		m.statsMu.Lock()
		m.dropped++
		m.statsMu.Unlock()
	}
	samples, ok := m.buf.TakeWindow()
	if !ok {
		return
	}
	wav := EncodeWAV(samples)
	asrStart := time.Now()
	text, err := m.trans.Transcribe(ctx, wav)
	asrDur := time.Since(asrStart)
	if err != nil {
		log.Printf("直播字幕识别失败: %v", err)
		m.setError("识别失败: " + err.Error())
		return
	}
	text = cleanTranscription(text)
	if text == "" {
		return
	}
	m.statsMu.Lock()
	dup := text == m.lastSource
	if !dup {
		m.lastSource = text
	}
	m.statsMu.Unlock()
	if dup {
		return
	}

	trStart := time.Now()
	out := text
	if m.cfg.Translator != nil {
		if zh, err := m.cfg.Translator.Translate(ctx, text); err == nil && strings.TrimSpace(zh) != "" {
			out = zh
		}
		// 翻译失败回退英文，不丢这条字幕。
	}
	trDur := time.Since(trStart)
	// 端到端延迟：窗口内最早那句话等了整窗 + 识别 + 翻译（保守口径）。
	latency := time.Duration(m.cfg.WindowSeconds*float64(time.Second)) + asrDur + trDur
	m.statsMu.Lock()
	m.processed++
	m.latencySum += latency
	m.lastLatency = latency
	m.lastASR = asrDur
	m.lastTranslate = trDur
	m.lastText = text
	m.lastSubtitle = out
	m.lastErr = ""
	m.statsMu.Unlock()
	if m.cfg.Emit != nil {
		m.cfg.Emit(out, latency)
	}
}

var (
	bracketRe = regexp.MustCompile(`\[[^\]]*\]`)
	parenRe   = regexp.MustCompile(`\([^)]*\)`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

// cleanTranscription 去掉 Whisper 的静音幻觉与标注：
// [BLANK_AUDIO]、[音乐]、(掌声) 这类方括号/圆括号内容；
// 剩下的空白压缩、去首尾。空了就返回 ""（调用方跳过这一窗）。
func cleanTranscription(s string) string {
	s = bracketRe.ReplaceAllString(s, " ")
	s = parenRe.ReplaceAllString(s, " ")
	s = spaceRe.ReplaceAllString(strings.TrimSpace(s), " ")
	return s
}
