// Package translate 的谷歌 Cloud Translation 翻译客户端。
//
// 用免费层（每月 50 万字符免费）做英译中，不用本地跑大模型：
// 调用方在超时/失败时跳过该行，翻译失败永远不挡播放。
// API Key 由用户在设置页填写（Google Cloud 控制台免费开，
// 启用 Cloud Translation API 即可，不收费），只存在本机，不上传别处。
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultGoogleTimeout 是免费接口单次调用超时：实时字幕不能等太久，
// 超时调用方跳过该行。
const DefaultGoogleTimeout = 6 * time.Second

// defaultGoogleEndpoint 是 Cloud Translation 入口；测试可注入 httptest 地址。
const defaultGoogleEndpoint = "https://translation.googleapis.com"

// GoogleClient 是谷歌 Cloud Translation 免费接口的翻译客户端。
type GoogleClient struct {
	key      string
	endpoint string
	http     *http.Client
	cache    *translationCache
}

// NewGoogleClient 构造客户端；endpoint 为空时用默认入口，
// timeout<=0 时用 DefaultGoogleTimeout。
func NewGoogleClient(key, endpoint string, timeout time.Duration) *GoogleClient {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultGoogleEndpoint
	}
	if timeout <= 0 {
		timeout = DefaultGoogleTimeout
	}
	return &GoogleClient{
		key:      strings.TrimSpace(key),
		endpoint: strings.TrimRight(endpoint, "/"),
		http:     &http.Client{Timeout: timeout},
		cache:    newTranslationCache(),
	}
}

// googleTranslateRequest 是 Cloud Translation 接口的请求体。
type googleTranslateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
	Format string `json:"format"`
}

// googleTranslateResponse 是 Cloud Translation 接口的响应体。
type googleTranslateResponse struct {
	Data struct {
		Translations []struct {
			TranslatedText string `json:"translatedText"`
		} `json:"translations"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Translate 把一条英文字幕翻成中文；同一条只翻一次（缓存），
// 空文本与中文不发请求，任何失败都返回错误（调用方回落或跳过）。
// 谷歌返回的译文可能带 HTML 转义（如 &amp;），先反转义再清洗。
func (c *GoogleClient) Translate(ctx context.Context, text string) (string, error) {
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
	if c.key == "" {
		return "", errors.New("未填 Google API Key")
	}

	reqBody, err := json.Marshal(googleTranslateRequest{
		Q:      text,
		Source: "en",
		Target: "zh-CN",
		Format: "text",
	})
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	u, err := url.Parse(c.endpoint + "/language/translate/v2")
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	q := u.Query()
	q.Set("key", c.key)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
	var out googleTranslateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("解析翻译结果失败: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("免费翻译接口返回错误: %s", out.Error.Message)
	}
	if len(out.Data.Translations) == 0 {
		return "", errors.New("免费翻译接口返回了空译文")
	}
	cleaned := cleanTranslation(html.UnescapeString(out.Data.Translations[0].TranslatedText))
	if cleaned == "" {
		return "", errors.New("免费翻译接口返回了空译文")
	}
	c.cache.put(text, cleaned)
	return cleaned, nil
}

// Check 用一句 Hello 探测 Key 是否可用：Key 错会直接报接口错误，
// 设置页据此提示用户去检查（免费层 Key 在 Google Cloud 控制台免费开）。
func (c *GoogleClient) Check(ctx context.Context) error {
	if c.key == "" {
		return errors.New("未填 Google API Key")
	}
	_, err := c.Translate(ctx, "Hello")
	return err
}
