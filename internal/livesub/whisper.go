package livesub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/httpx"
)

// WhisperClient 调 whisper-server（常驻子进程，模型只加载一次）：
// POST multipart 到 /inference，file 字段传 16kHz 单声道 WAV，
// language 固定 en（省掉语言检测，又快又准）。
type WhisperClient struct {
	endpoint string
	http     *http.Client
}

// NewWhisperClient 构造客户端；endpoint 是 whisper-server 根地址
// （如 http://127.0.0.1:17880），timeout<=0 时默认 30 秒。
func NewWhisperClient(endpoint string, timeout time.Duration) *WhisperClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &WhisperClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		// whisper-server 默认在本机回环上，代理选择对它直连（回环不进代理），
		// 行为不变；统一走 httpx 是为以后 endpoint 配到远端时也不直连。
		http: httpx.NewClient(timeout),
	}
}

type whisperInferenceResponse struct {
	Text string `json:"text"`
}

// Transcribe 把一段 WAV 音频发给 whisper-server，返回识别出的英文文本。
// 空音频不发请求；任何失败都返回错误（调用方直接丢掉这一窗，不挡后续）。
func (c *WhisperClient) Transcribe(ctx context.Context, wav []byte) (string, error) {
	if len(wav) == 0 {
		return "", fmt.Errorf("音频为空")
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("构造识别请求失败: %w", err)
	}
	if _, err := fw.Write(wav); err != nil {
		return "", fmt.Errorf("构造识别请求失败: %w", err)
	}
	if err := mw.WriteField("language", "en"); err != nil {
		return "", fmt.Errorf("构造识别请求失败: %w", err)
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("构造识别请求失败: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("构造识别请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/inference", &body)
	if err != nil {
		return "", fmt.Errorf("构造识别请求失败: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求语音识别服务失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return "", fmt.Errorf("语音识别服务返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("读取识别结果失败: %w", err)
	}
	var out whisperInferenceResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析识别结果失败: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}

// WaitReady 等 whisper-server 就绪：轮询根路径，
// 拿到 200 即认为服务已起；超时返回错误。
func (c *WhisperClient) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/", nil)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("语音识别服务未就绪（%s）", c.endpoint)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
