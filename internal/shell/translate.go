// Package shell 的本地字幕翻译胶水：把 internal/translate 的 Ollama 客户端
// 暴露给前端，并把开关/模型/地址持久化到 store.kv。
//
// 客户端按「地址+模型」缓存复用：字幕逐条翻译时同一条只翻一次
// （缓存由 translate.Client 维护），每次调用都新建客户端会丢掉缓存。
package shell

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/translate"
)

// 翻译设置在 store.kv 里的键（与播放设置 playback.* 同一命名风格）。
const (
	translateEnabledKey  = "translate.enabled"
	translateModelKey    = "translate.model"
	translateEndpointKey = "translate.endpoint"
)

// TranslateSettings 是本地字幕翻译选项：只做英译中，默认关闭。
type TranslateSettings struct {
	Enabled  bool
	Model    string
	Endpoint string
}

// GetTranslateSettings 返回字幕翻译设置；未设置或非法时回退默认值。
func (s *ShellService) GetTranslateSettings() TranslateSettings {
	settings := TranslateSettings{
		Enabled:  false,
		Model:    translate.DefaultModel,
		Endpoint: translate.DefaultEndpoint,
	}
	if s.store == nil {
		return settings
	}
	settings.Enabled = s.getPlaybackSetting(translateEnabledKey)
	if v, ok, err := s.store.GetKV(translateModelKey); err == nil && ok && strings.TrimSpace(v) != "" {
		settings.Model = strings.TrimSpace(v)
	}
	if v, ok, err := s.store.GetKV(translateEndpointKey); err == nil && ok && strings.TrimSpace(v) != "" {
		settings.Endpoint = strings.TrimSpace(v)
	}
	return settings
}

// SetTranslateSettings 持久化字幕翻译设置；模型/地址留空时读回默认值。
func (s *ShellService) SetTranslateSettings(settings TranslateSettings) error {
	if s.store == nil {
		return nil
	}
	for _, kv := range []struct {
		key   string
		value string
	}{
		{translateEnabledKey, strconv.FormatBool(settings.Enabled)},
		{translateModelKey, strings.TrimSpace(settings.Model)},
		{translateEndpointKey, strings.TrimSpace(settings.Endpoint)},
	} {
		if err := s.store.SetKV(kv.key, kv.value); err != nil {
			return err
		}
	}
	return nil
}

// TranslateSubtitle 把一条英文字幕翻成中文。开关关闭或空文本时直接返回原文；
// 翻译失败返回错误，调用方（前端）回退显示原文、绝不挡播放。
func (s *ShellService) TranslateSubtitle(text string) (string, error) {
	settings := s.GetTranslateSettings()
	if !settings.Enabled {
		return text, nil
	}
	if strings.TrimSpace(text) == "" {
		return text, nil
	}
	return translateClientFor(settings.Endpoint, settings.Model).Translate(context.Background(), text)
}

// CheckTranslateModel 检查本地是否已拉取当前翻译模型；
// 服务没起或模型未拉取都返回中文错误，设置页据此提示用户。
func (s *ShellService) CheckTranslateModel() error {
	settings := s.GetTranslateSettings()
	return translateClientFor(settings.Endpoint, settings.Model).CheckModel(context.Background())
}

var (
	translateClientMu sync.Mutex
	translateClients  = map[string]*translate.Client{}
)

// translateClientFor 按「地址+模型」复用客户端；设置改了才新建。
func translateClientFor(endpoint, model string) *translate.Client {
	key := strings.TrimSpace(endpoint) + "\x00" + strings.TrimSpace(model)
	translateClientMu.Lock()
	defer translateClientMu.Unlock()
	if c, ok := translateClients[key]; ok {
		return c
	}
	c := translate.NewClient(endpoint, model, 0)
	translateClients[key] = c
	return c
}
