// Package translate 用本地大模型（Ollama 兼容接口）逐条翻译字幕文本。
//
// 只负责英译中、单条短文本：调用方（播放器）逐条传字幕原文，拿到译文就显示，
// 拿不到（模型没装、服务没起、超时、返回空）就返回错误、调用方回退原文，
// 翻译失败永远不挡播放。
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
)

// DefaultEndpoint 是 Ollama 默认监听地址。
const DefaultEndpoint = "http://127.0.0.1:11434"

// DefaultModel 是默认翻译模型：Qwen2.5-3B 指令版，下载约 1.9GB、
// 加载约 2.3GB 内存，字幕短句在 GPU 上约 0.5 秒/条、纯 CPU 约 1–2 秒/条。
const DefaultModel = "qwen2.5:3b"

// maxCacheEntries 是翻译缓存上限：字幕去重后条目有限，超了就清空重记，
// 避免长片看下来缓存无限膨胀。
const maxCacheEntries = 500

// systemPrompt 要求模型只输出译文：短句翻译不需要解释，
// 解释性文字会直接混进字幕显示出来。
const systemPrompt = "You are a subtitle translator. Translate the following English subtitle line into natural, concise Chinese. Output ONLY the Chinese translation, no explanation, no quotes, no preamble."

// Client 是 Ollama 兼容接口的翻译客户端。
type Client struct {
	endpoint string
	model    string
	http     *http.Client
	mu       sync.Mutex
	cache    map[string]string
	order    []string
}

// NewClient 构造客户端；endpoint 为空时用 DefaultEndpoint，model 为空时用 DefaultModel。
func NewClient(endpoint, model string, timeout time.Duration) *Client {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		http:     &http.Client{Timeout: timeout},
		cache:    make(map[string]string),
	}
}

// chatRequest 是 /api/chat 的请求体（stream 必须显式 false，
// Ollama 默认 stream 为 true，会返回 NDJSON 流）。
type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// Translate 把一条英文字幕翻成中文；同一条只翻一次（缓存），
// 空文本不发请求，任何失败都返回错误（调用方回退原文）。
func (c *Client) Translate(ctx context.Context, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if isChineseText(text) {
		return text, nil
	}

	c.mu.Lock()
	if got, ok := c.cache[text]; ok {
		c.mu.Unlock()
		return got, nil
	}
	c.mu.Unlock()

	reqBody, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
		Stream:  false,
		Options: map[string]any{"temperature": 0.2},
	})
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求本地模型失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("本地模型返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("读取翻译结果失败: %w", err)
	}
	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("解析翻译结果失败: %w", err)
	}
	cleaned := cleanTranslation(out.Message.Content)
	if cleaned == "" {
		return "", fmt.Errorf("本地模型返回了空译文")
	}

	c.mu.Lock()
	if len(c.order) >= maxCacheEntries {
		c.cache = make(map[string]string, maxCacheEntries)
		c.order = c.order[:0]
	}
	if _, ok := c.cache[text]; !ok {
		c.order = append(c.order, text)
	}
	c.cache[text] = cleaned
	c.mu.Unlock()
	return cleaned, nil
}

// tagsResponse 是 /api/tags 的响应体。
type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// CheckModel 检查本地 Ollama 是否已拉取当前模型：服务没起、
// 模型没拉取都返回错误（设置页据此提示用户先装模型）。
func (c *Client) CheckModel(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("构造模型检查请求失败: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("连接本地模型服务失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("本地模型服务返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取模型列表失败: %w", err)
	}
	var out tagsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("解析模型列表失败: %w", err)
	}
	for _, m := range out.Models {
		if m.Name == c.model || strings.HasPrefix(m.Name, c.model+":") {
			return nil
		}
	}
	return fmt.Errorf("本地未找到模型 %s，请先执行 ollama pull %s", c.model, c.model)
}

// cleanTranslation 去掉模型常带的多余包装：首尾空白、
// 整句引号（英文 " 与中文 “”）、「翻译：/Translation:」前缀。
func cleanTranslation(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"翻译：", "翻译:", "Translation:", "translation:"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
			break
		}
	}
	if len(s) >= 2 {
		runes := []rune(s)
		first, last := runes[0], runes[len(runes)-1]
		if (first == '"' && last == '"') || (first == '“' && last == '”') {
			s = strings.TrimSpace(string(runes[1 : len(runes)-1]))
		}
	}
	return s
}

// isChineseText 判一段文本是否含中文：含中文就不翻译
// （已是中文或双语字幕直接显示原文）。
func isChineseText(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
