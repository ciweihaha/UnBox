// Package translate 的微软 Azure Translator 翻译客户端。
//
// 用免费层 F0（每月 200 万字符免费，实时字幕用量绰绰有余）做英译中，
// 不用本地跑大模型：调用方在超时/失败时跳过该行，
// 翻译失败永远不挡播放。API Key 由用户在设置页填写
// （Azure 门户免费开 F0 资源即可，不收费），只存在本机，不上传别处。
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

// DefaultAzureTimeout 是免费接口单次调用超时：实时字幕不能等太久，
// 超时调用方跳过该行。
const DefaultAzureTimeout = 6 * time.Second

// defaultAzureEndpoint 是 Translator 全球入口；测试可注入 httptest 地址。
const defaultAzureEndpoint = "https://api.cognitive.microsofttranslator.com"

// AzureClient 是 Azure Translator 免费接口的翻译客户端。
type AzureClient struct {
	key      string
	region   string
	endpoint string
	http     *http.Client
	cache    *translationCache
}

// NewAzureClient 构造客户端；endpoint 为空时用全球入口，
// timeout<=0 时用 DefaultAzureTimeout。region 可空（全球入口可不用区域），
// 填了就随请求带上（部分订阅要求带区域）。
func NewAzureClient(key, region, endpoint string, timeout time.Duration) *AzureClient {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultAzureEndpoint
	}
	if timeout <= 0 {
		timeout = DefaultAzureTimeout
	}
	return &AzureClient{
		key:      strings.TrimSpace(key),
		region:   strings.TrimSpace(region),
		endpoint: strings.TrimRight(endpoint, "/"),
		// 翻译走外网 API：传输层用 httpx.NewClient（环境变量优先，
		// 其次 Windows 系统代理），不能用裸 http.Client 直连。
		http:  httpx.NewClient(timeout),
		cache: newTranslationCache(),
	}
}

// azureTranslateRequest 是 Translator 接口的请求体（一批一条）。
type azureTranslateRequest struct {
	Text string `json:"Text"`
}

// azureTranslateResponse 是 Translator 接口的响应体。
type azureTranslateResponse struct {
	Translations []struct {
		Text string `json:"text"`
		To   string `json:"to"`
	} `json:"translations"`
}

// Translate 把一条英文字幕翻成中文；同一条只翻一次（缓存），
// 空文本与中文不发请求，任何失败都返回错误（调用方回落或跳过）。
func (c *AzureClient) Translate(ctx context.Context, text string) (string, error) {
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
		return "", errors.New("未填 Azure API Key")
	}

	reqBody, err := json.Marshal([]azureTranslateRequest{{Text: text}})
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	url := c.endpoint + "/translate?api-version=3.0&from=en&to=zh-Hans"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("构造翻译请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ocp-Apim-Subscription-Key", c.key)
	if c.region != "" {
		req.Header.Set("Ocp-Apim-Subscription-Region", c.region)
	}

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
	var out []azureTranslateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("解析翻译结果失败: %w", err)
	}
	if len(out) == 0 || len(out[0].Translations) == 0 {
		return "", errors.New("免费翻译接口返回了空译文")
	}
	cleaned := cleanTranslation(out[0].Translations[0].Text)
	if cleaned == "" {
		return "", errors.New("免费翻译接口返回了空译文")
	}
	c.cache.put(text, cleaned)
	return cleaned, nil
}

// Check 用一句 Hello 探测 Key 是否可用：Key 错会直接报接口错误，
// 设置页据此提示用户去检查（F0 免费层 Key 在 Azure 门户免费开）。
func (c *AzureClient) Check(ctx context.Context) error {
	if c.key == "" {
		return errors.New("未填 Azure API Key")
	}
	_, err := c.Translate(ctx, "Hello")
	return err
}
