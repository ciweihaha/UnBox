// Package translate 的 Cloudflare Workers AI 翻译客户端。
//
// 用免费层（每天 10000 neurons）跑 @cf/meta/m2m100-1.2b 翻译模型，
// 不用本地跑大模型：调用方在超时/失败时跳过该行，
// 翻译失败永远不挡播放。账号 ID 与 API 令牌由用户在设置页填写
// （令牌只需 Workers AI 的运行权限），只存在本机，不上传别处。
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/httpx"
)

// DefaultCFModel 是免费接口默认翻译模型：Meta 的多语言翻译模型，
// 英译中走 source_lang=english、target_lang=chinese。
const DefaultCFModel = "@cf/meta/m2m100-1.2b"

// DefaultCFTimeout 是免费接口单次调用超时：实时字幕不能等太久，
// 超时调用方跳过该行。
const DefaultCFTimeout = 6 * time.Second

// defaultCFAPIBase 是 Cloudflare API 入口；测试可注入 httptest 地址。
const defaultCFAPIBase = "https://api.cloudflare.com/client/v4"

// CFClient 是 Workers AI 免费接口的翻译客户端。
type CFClient struct {
	accountID string
	token     string
	model     string
	baseURL   string
	http      *http.Client
	cache     *translationCache
}

// NewCFClient 构造客户端；baseURL 为空时用 Cloudflare API 入口，
// model 为空时用 DefaultCFModel，timeout<=0 时用 DefaultCFTimeout。
func NewCFClient(accountID, token, baseURL string, timeout time.Duration) *CFClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultCFAPIBase
	}
	if timeout <= 0 {
		timeout = DefaultCFTimeout
	}
	return &CFClient{
		accountID: strings.TrimSpace(accountID),
		token:     strings.TrimSpace(token),
		model:     DefaultCFModel,
		baseURL:   strings.TrimRight(baseURL, "/"),
		// 翻译走外网 API：传输层用 httpx.NewClient（环境变量优先，
		// 其次 Windows 系统代理），不能用裸 http.Client 直连。
		http:  httpx.NewClient(timeout),
		cache: newTranslationCache(),
	}
}

// cfTranslateRequest 是 Workers AI 翻译接口的请求体。
type cfTranslateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// cfTranslateResponse 是 Workers AI 翻译接口的响应体。
type cfTranslateResponse struct {
	Success bool `json:"success"`
	Result  struct {
		TranslatedText string `json:"translated_text"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Translate 把一条英文字幕翻成中文；同一条只翻一次（缓存），
// 空文本与中文不发请求，任何失败都返回错误（调用方回落或跳过）。
func (c *CFClient) Translate(ctx context.Context, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if isChineseText(text) {
		return text, nil
	}
	if got, ok := c.cache.get(text); ok {
		return got, nil
	}
	if c.accountID == "" || c.token == "" {
		return "", errors.New("未填 Cloudflare 账号 ID 或 API 令牌")
	}

	reqBody, err := json.Marshal(cfTranslateRequest{
		Text:       text,
		SourceLang: "english",
		TargetLang: "chinese",
	})
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	url := c.baseURL + "/accounts/" + c.accountID + "/ai/run/" + c.model
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求免费翻译接口失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("读取翻译结果失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("免费翻译接口返回 HTTP %d", resp.StatusCode)
	}
	var out cfTranslateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("解析翻译结果失败: %w", err)
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		if msg != "" {
			return "", fmt.Errorf("免费翻译接口返回错误: %s", msg)
		}
		return "", errors.New("免费翻译接口返回错误")
	}
	cleaned := cleanTranslation(out.Result.TranslatedText)
	if cleaned == "" {
		return "", errors.New("免费翻译接口返回了空译文")
	}
	c.cache.put(text, cleaned)
	return cleaned, nil
}

// Check 用一句 Hello 探测账号与令牌是否可用：账号/令牌错会直接报接口错误，
// 设置页据此提示用户去检查（令牌需要 Workers AI 的运行权限）。
func (c *CFClient) Check(ctx context.Context) error {
	if c.accountID == "" || c.token == "" {
		return errors.New("未填 Cloudflare 账号 ID 或 API 令牌")
	}
	_, err := c.Translate(ctx, "Hello")
	return err
}
