// Package shell 的字幕翻译胶水：把 internal/translate 的免费接口翻译客户端
// 暴露给前端，并把翻译设置持久化到 store.kv。
//
// 翻译只走免费接口（Provider=free），来源三选一（FreeProvider）：
// 微软 Azure Translator 免费层 F0（每月 200 万字符，默认）、
// 谷歌 Cloud Translation 免费层（每月 50 万字符）、
// Cloudflare Workers AI 免费层（每天 10000 neurons）。
// 本地大模型已不用（2026-11-19 用户决定：效果不好，整档删掉）：
// 接口超时或失败就返回错误，调用方跳过该行字幕、显示英文原文，
// 不再回落本地模型。Whisper 听写仍走本地（那是听写不是翻译，不受影响）。
// 客户端按「Key/区域」「账号+令牌」缓存复用：字幕逐条翻译时同一条只翻一次
// （缓存由各 translate 客户端维护），每次调用都新建客户端会丢掉缓存。
package shell

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/livesub"
	"github.com/unbox/unbox/internal/translate"
)

// 翻译开关：免费接口、关闭（本地模型已删除）。
const (
	ProviderFree = "free"
	ProviderOff  = "off"
)

// 免费接口服务商：微软 Azure（默认）、谷歌、Cloudflare。
const (
	FreeAzure      = "azure"
	FreeGoogle     = "google"
	FreeCloudflare = "cloudflare"
)

// 翻译设置在 store.kv 里的键（与播放设置 playback.* 同一命名风格）。
const (
	translateEnabledKey      = "translate.enabled"
	translateProviderKey     = "translate.provider"
	translateFreeProviderKey = "translate.free.provider"
	translateFreeAPIKeyKey   = "translate.free.apiKey"
	translateAzureRegionKey  = "translate.azure.region"
	translateCFAccountKey    = "translate.cf.accountId"
	translateCFTokenKey      = "translate.cf.token"
)

// 免费接口的 API 入口；为空时用各客户端的默认入口，
// 测试可注入 httptest 地址（与 liveSubRetryDelay 同一做法）。
var (
	translateCFBaseURL     = ""
	translateAzureBaseURL  = ""
	translateGoogleBaseURL = ""
)

// TranslateSettings 是字幕翻译选项：只做英译中，默认关闭。
// FreeAPIKey 是 Azure / 谷歌的 API Key（用户自己去微软/谷歌免费开，
// F0 与免费层都不收费）；Cloudflare 用 CFAccountID + CFToken。
type TranslateSettings struct {
	Enabled      bool
	Provider     string
	FreeProvider string
	FreeAPIKey   string
	AzureRegion  string
	CFAccountID  string
	CFToken      string
}

// normalizeTranslateProvider 把来源归一化到二选一；
// 旧值 ollama（本地模型已删）与 cloudflare（现为免费接口下的服务商）
// 都迁到免费接口；空值或非法值按 Enabled 兜底（开→免费接口，关→关闭）。
func normalizeTranslateProvider(provider string, enabled bool) string {
	switch strings.TrimSpace(provider) {
	case ProviderFree, ProviderOff:
		return strings.TrimSpace(provider)
	case "ollama", "cloudflare":
		return ProviderFree
	default:
		if enabled {
			return ProviderFree
		}
		return ProviderOff
	}
}

// normalizeFreeProvider 把免费接口服务商归一化；空值或非法值默认 Azure。
func normalizeFreeProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case FreeAzure, FreeGoogle, FreeCloudflare:
		return strings.TrimSpace(provider)
	default:
		return FreeAzure
	}
}

// GetTranslateSettings 返回字幕翻译设置；未设置或非法时回退默认值。
// 旧版只有 translate.enabled（无 provider 键）时按开关迁移：
// 开→免费接口（Azure 默认），关→关闭。
func (s *ShellService) GetTranslateSettings() TranslateSettings {
	settings := TranslateSettings{
		Enabled:      false,
		Provider:     ProviderOff,
		FreeProvider: FreeAzure,
	}
	if s.store == nil {
		return settings
	}
	settings.Enabled = s.getPlaybackSetting(translateEnabledKey)
	provider := ""
	if v, ok, err := s.store.GetKV(translateProviderKey); err == nil && ok {
		provider = v
	}
	settings.Provider = normalizeTranslateProvider(provider, settings.Enabled)
	settings.Enabled = settings.Provider != ProviderOff
	if v, ok, err := s.store.GetKV(translateFreeProviderKey); err == nil && ok {
		settings.FreeProvider = normalizeFreeProvider(v)
	}
	// 旧版 provider=cloudflare（无 free.provider 键）时服务商迁到 Cloudflare。
	if provider == "cloudflare" {
		if _, ok, err := s.store.GetKV(translateFreeProviderKey); err == nil && !ok {
			settings.FreeProvider = FreeCloudflare
		}
	}
	if v, ok, err := s.store.GetKV(translateFreeAPIKeyKey); err == nil && ok {
		settings.FreeAPIKey = strings.TrimSpace(v)
	}
	if v, ok, err := s.store.GetKV(translateAzureRegionKey); err == nil && ok {
		settings.AzureRegion = strings.TrimSpace(v)
	}
	if v, ok, err := s.store.GetKV(translateCFAccountKey); err == nil && ok {
		settings.CFAccountID = strings.TrimSpace(v)
	}
	if v, ok, err := s.store.GetKV(translateCFTokenKey); err == nil && ok {
		settings.CFToken = strings.TrimSpace(v)
	}
	return settings
}

// SetTranslateSettings 持久化字幕翻译设置；来源为空时按 Enabled 归一化
// （开→免费接口，关→关闭），Enabled 始终与来源一致。
func (s *ShellService) SetTranslateSettings(settings TranslateSettings) error {
	settings.Provider = normalizeTranslateProvider(settings.Provider, settings.Enabled)
	settings.FreeProvider = normalizeFreeProvider(settings.FreeProvider)
	settings.Enabled = settings.Provider != ProviderOff
	if s.store == nil {
		return nil
	}
	for _, kv := range []struct {
		key   string
		value string
	}{
		{translateEnabledKey, strconv.FormatBool(settings.Enabled)},
		{translateProviderKey, settings.Provider},
		{translateFreeProviderKey, settings.FreeProvider},
		{translateFreeAPIKeyKey, strings.TrimSpace(settings.FreeAPIKey)},
		{translateAzureRegionKey, strings.TrimSpace(settings.AzureRegion)},
		{translateCFAccountKey, strings.TrimSpace(settings.CFAccountID)},
		{translateCFTokenKey, strings.TrimSpace(settings.CFToken)},
	} {
		if err := s.store.SetKV(kv.key, kv.value); err != nil {
			return err
		}
	}
	return nil
}

// TranslateSubtitle 把一条英文字幕翻成中文。开关关闭或空文本时直接返回原文；
// 接口失败返回错误，调用方（前端）跳过该行、显示原文，绝不挡播放。
func (s *ShellService) TranslateSubtitle(text string) (string, error) {
	settings := s.GetTranslateSettings()
	if settings.Provider == ProviderOff {
		return text, nil
	}
	if strings.TrimSpace(text) == "" {
		return text, nil
	}
	return s.translateWithProvider(context.Background(), settings, text)
}

// translateWithProvider 按翻译设置翻译一条文本：只走所选的免费接口，
// 失败不回落（本地模型已删），直接返回错误由调用方跳过该行。
func (s *ShellService) translateWithProvider(ctx context.Context, settings TranslateSettings, text string) (string, error) {
	switch settings.FreeProvider {
	case FreeGoogle:
		return googleClientFor(settings.FreeAPIKey).Translate(ctx, text)
	case FreeCloudflare:
		return cfClientFor(settings.CFAccountID, settings.CFToken).Translate(ctx, text)
	default:
		return azureClientFor(settings.FreeAPIKey, settings.AzureRegion).Translate(ctx, text)
	}
}

// CheckTranslateModel 检查当前翻译来源是否可用：免费接口用一句 Hello
// 探测 Key/账号令牌；Key 错、令牌错、服务没起都返回中文错误，
// 设置页据此提示用户去检查。
func (s *ShellService) CheckTranslateModel() error {
	settings := s.GetTranslateSettings()
	if settings.Provider == ProviderOff {
		return errors.New("字幕翻译已关闭")
	}
	switch settings.FreeProvider {
	case FreeGoogle:
		return googleClientFor(settings.FreeAPIKey).Check(context.Background())
	case FreeCloudflare:
		return cfClientFor(settings.CFAccountID, settings.CFToken).Check(context.Background())
	default:
		return azureClientFor(settings.FreeAPIKey, settings.AzureRegion).Check(context.Background())
	}
}

// translatorFor 按翻译设置构造直播实时字幕的翻译器：关闭时返回 nil
// （管线直接出英文原文）；其余走同一条免费接口翻译路径。
func (s *ShellService) translatorFor(settings TranslateSettings) livesub.Translator {
	if settings.Provider == ProviderOff {
		return nil
	}
	return &settingsTranslator{svc: s, settings: settings}
}

// settingsTranslator 把当前翻译设置适配成 livesub.Translator。
type settingsTranslator struct {
	svc      *ShellService
	settings TranslateSettings
}

func (t *settingsTranslator) Translate(ctx context.Context, text string) (string, error) {
	return t.svc.translateWithProvider(ctx, t.settings, text)
}

var (
	freeTranslateClientMu  sync.Mutex
	azureTranslateClients  = map[string]*translate.AzureClient{}
	googleTranslateClients = map[string]*translate.GoogleClient{}
	cfTranslateClients     = map[string]*translate.CFClient{}
)

// azureClientFor 按「Key+区域」复用 Azure 免费接口客户端；设置改了才新建。
func azureClientFor(apiKey, region string) *translate.AzureClient {
	key := strings.TrimSpace(apiKey) + "\x00" + strings.TrimSpace(region)
	freeTranslateClientMu.Lock()
	defer freeTranslateClientMu.Unlock()
	if c, ok := azureTranslateClients[key]; ok {
		return c
	}
	c := translate.NewAzureClient(apiKey, region, translateAzureBaseURL, 0)
	azureTranslateClients[key] = c
	return c
}

// googleClientFor 按 Key 复用谷歌免费接口客户端；设置改了才新建。
func googleClientFor(apiKey string) *translate.GoogleClient {
	key := strings.TrimSpace(apiKey)
	freeTranslateClientMu.Lock()
	defer freeTranslateClientMu.Unlock()
	if c, ok := googleTranslateClients[key]; ok {
		return c
	}
	c := translate.NewGoogleClient(apiKey, translateGoogleBaseURL, 0)
	googleTranslateClients[key] = c
	return c
}

// cfClientFor 按「账号+令牌」复用 Cloudflare 免费接口客户端；设置改了才新建。
func cfClientFor(accountID, token string) *translate.CFClient {
	key := strings.TrimSpace(accountID) + "\x00" + strings.TrimSpace(token)
	freeTranslateClientMu.Lock()
	defer freeTranslateClientMu.Unlock()
	if c, ok := cfTranslateClients[key]; ok {
		return c
	}
	c := translate.NewCFClient(accountID, token, translateCFBaseURL, 0)
	cfTranslateClients[key] = c
	return c
}
