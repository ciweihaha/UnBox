package shell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/store"
)

// newTranslateService 建一个带真实 store 的服务实例（复用播放设置的测试辅助）。
func newTranslateService(t *testing.T) (*ShellService, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/translate.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return newPlaybackSettingsService(t, st), st
}

// newAzureStub 模拟 Azure Translator 接口：校验 Key，返回固定译文或指定状态。
// 每个测试用不同 Key，避免 azureClientFor 的客户端缓存互相串用。
func newAzureStub(t *testing.T, key, translated string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != key {
			t.Errorf("Ocp-Apim-Subscription-Key = %q，想要 %q", got, key)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"translations": []map[string]any{{"text": translated, "to": "zh-Hans"}}},
		})
	}))
}

// useAzureBase 把 Azure 接口入口指向测试服务器，测试结束恢复。
func useAzureBase(t *testing.T, url string) {
	t.Helper()
	old := translateAzureBaseURL
	translateAzureBaseURL = url
	t.Cleanup(func() { translateAzureBaseURL = old })
}

// newGoogleStub 模拟 Cloud Translation 接口：校验 Key，返回固定译文或指定状态。
func newGoogleStub(t *testing.T, key, translated string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != key {
			t.Errorf("key 参数 = %q，想要 %q", got, key)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"translations": []map[string]any{{"translatedText": translated}},
			},
		})
	}))
}

// useGoogleBase 把谷歌接口入口指向测试服务器，测试结束恢复。
func useGoogleBase(t *testing.T, url string) {
	t.Helper()
	old := translateGoogleBaseURL
	translateGoogleBaseURL = url
	t.Cleanup(func() { translateGoogleBaseURL = old })
}

// newCFStub 模拟 Workers AI 翻译接口：路径与账号对应，返回固定译文或指定状态。
// 每个测试用不同账号，避免 cfClientFor 的客户端缓存互相串用。
func newCFStub(t *testing.T, account, translated string, status int, success bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/accounts/" + account + "/ai/run/@cf/meta/m2m100-1.2b"
		if r.URL.Path != want {
			t.Errorf("请求路径 = %s，想要 %s", r.URL.Path, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok-abc" {
			t.Errorf("Authorization = %q，想要 Bearer tok-abc", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": success,
			"result":  map[string]any{"translated_text": translated},
			"errors":  []any{},
		})
	}))
}

// useCFBase 把免费接口入口指向测试服务器，测试结束恢复。
func useCFBase(t *testing.T, url string) {
	t.Helper()
	old := translateCFBaseURL
	translateCFBaseURL = url
	t.Cleanup(func() { translateCFBaseURL = old })
}

func TestTranslateSettingsDefaults(t *testing.T) {
	svc, _ := newTranslateService(t)
	got := svc.GetTranslateSettings()
	if got.Enabled {
		t.Fatalf("默认 Enabled = true， want false（翻译默认关）")
	}
	if got.Provider != ProviderOff {
		t.Fatalf("默认 Provider = %q， want off", got.Provider)
	}
	if got.FreeProvider != FreeAzure {
		t.Fatalf("默认 FreeProvider = %q， want azure", got.FreeProvider)
	}
}

// 免费接口（Azure）设置与 Key、区域要能持久化、重启后读回。
func TestTranslateSettingsFreeAzureRoundTrip(t *testing.T) {
	svc, st := newTranslateService(t)
	want := TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeAzure, FreeAPIKey: "azure-key-1", AzureRegion: "eastasia"}
	if err := svc.SetTranslateSettings(want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetTranslateSettings(); got != want {
		t.Fatalf("round trip = %#v， want %#v", got, want)
	}
}

// 免费接口（谷歌）设置与 Key 要能持久化、重启后读回。
func TestTranslateSettingsFreeGoogleRoundTrip(t *testing.T) {
	svc, st := newTranslateService(t)
	want := TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeGoogle, FreeAPIKey: "google-key-1"}
	if err := svc.SetTranslateSettings(want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetTranslateSettings(); got != want {
		t.Fatalf("round trip = %#v， want %#v", got, want)
	}
}

// 免费接口（Cloudflare）设置与账号令牌要能持久化、重启后读回。
func TestTranslateSettingsFreeCloudflareRoundTrip(t *testing.T) {
	svc, st := newTranslateService(t)
	want := TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeCloudflare, CFAccountID: "acct-123", CFToken: "tok-abc"}
	if err := svc.SetTranslateSettings(want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetTranslateSettings(); got != want {
		t.Fatalf("round trip = %#v， want %#v", got, want)
	}
}

// 旧版只有 translate.enabled（无 provider 键）时按开关迁移：
// 开→免费接口（Azure 默认），关→关闭；老用户升级后翻译走免费接口。
func TestTranslateSettingsLegacyEnabledMigrates(t *testing.T) {
	svc, st := newTranslateService(t)
	if err := st.SetKV("translate.enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetTranslateSettings(); got.Provider != ProviderFree || got.FreeProvider != FreeAzure || !got.Enabled {
		t.Fatalf("旧开关开时 = %#v， want free/azure/true", got)
	}
	if err := st.SetKV("translate.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetTranslateSettings(); got.Provider != ProviderOff || got.Enabled {
		t.Fatalf("旧开关关时 Provider = %q Enabled = %v， want off/false", got.Provider, got.Enabled)
	}
}

// 旧版 provider=ollama（本地模型已删）迁到免费接口；
// 旧版 provider=cloudflare 迁到免费接口 + Cloudflare 服务商。
func TestTranslateSettingsLegacyProviderMigrates(t *testing.T) {
	svc, st := newTranslateService(t)
	if err := st.SetKV("translate.provider", "ollama"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetTranslateSettings(); got.Provider != ProviderFree || got.FreeProvider != FreeAzure {
		t.Fatalf("旧 ollama = %#v， want free/azure", got)
	}
	if err := st.SetKV("translate.provider", "cloudflare"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetTranslateSettings(); got.Provider != ProviderFree || got.FreeProvider != FreeCloudflare {
		t.Fatalf("旧 cloudflare = %#v， want free/cloudflare", got)
	}
}

// 来源选关闭时直接返回原文，不发任何请求。
func TestTranslateSubtitleOffReturnsOriginal(t *testing.T) {
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: false, Provider: ProviderOff}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.TranslateSubtitle("Hello")
	if err != nil {
		t.Fatalf("关闭时 TranslateSubtitle err = %v， want nil", err)
	}
	if got != "Hello" {
		t.Fatalf("关闭时 TranslateSubtitle = %q， want 原文 Hello", got)
	}
}

func TestTranslateSubtitleFreeAzureTranslates(t *testing.T) {
	stub := newAzureStub(t, "azure-key-ok", "你好，世界", http.StatusOK)
	t.Cleanup(stub.Close)
	useAzureBase(t, stub.URL)
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeAzure, FreeAPIKey: "azure-key-ok"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.TranslateSubtitle("Hello, world")
	if err != nil {
		t.Fatalf("TranslateSubtitle err = %v， want nil", err)
	}
	if got != "你好，世界" {
		t.Fatalf("TranslateSubtitle = %q， want 你好，世界", got)
	}
}

func TestTranslateSubtitleFreeGoogleTranslates(t *testing.T) {
	stub := newGoogleStub(t, "google-key-ok", "你好，世界", http.StatusOK)
	t.Cleanup(stub.Close)
	useGoogleBase(t, stub.URL)
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeGoogle, FreeAPIKey: "google-key-ok"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.TranslateSubtitle("Hello, world")
	if err != nil {
		t.Fatalf("TranslateSubtitle err = %v， want nil", err)
	}
	if got != "你好，世界" {
		t.Fatalf("TranslateSubtitle = %q， want 你好，世界", got)
	}
}

func TestTranslateSubtitleFreeCloudflareTranslates(t *testing.T) {
	stub := newCFStub(t, "acct-ok", "你好，世界", http.StatusOK, true)
	t.Cleanup(stub.Close)
	useCFBase(t, stub.URL)
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeCloudflare, CFAccountID: "acct-ok", CFToken: "tok-abc"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.TranslateSubtitle("Hello, world")
	if err != nil {
		t.Fatalf("TranslateSubtitle err = %v， want nil", err)
	}
	if got != "你好，世界" {
		t.Fatalf("TranslateSubtitle = %q， want 你好，世界", got)
	}
}

// 免费接口失败时返回错误（调用方跳过该行、显示原文），不再回落本地模型。
func TestTranslateSubtitleFreeFailureReturnsError(t *testing.T) {
	stub := newAzureStub(t, "azure-key-bad", "", http.StatusForbidden)
	t.Cleanup(stub.Close)
	useAzureBase(t, stub.URL)
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeAzure, FreeAPIKey: "azure-key-bad"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TranslateSubtitle("Hello"); err == nil {
		t.Fatal("接口 403 时 TranslateSubtitle err = nil， want 错误（调用方据此跳过该行）")
	}
}

func TestCheckTranslateModelFree(t *testing.T) {
	t.Run("Azure 接口就绪", func(t *testing.T) {
		stub := newAzureStub(t, "azure-key-check-ok", "你好", http.StatusOK)
		t.Cleanup(stub.Close)
		useAzureBase(t, stub.URL)
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeAzure, FreeAPIKey: "azure-key-check-ok"}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err != nil {
			t.Fatalf("CheckTranslateModel err = %v， want nil", err)
		}
	})

	t.Run("Azure Key 不对", func(t *testing.T) {
		stub := newAzureStub(t, "azure-key-check-bad", "", http.StatusForbidden)
		t.Cleanup(stub.Close)
		useAzureBase(t, stub.URL)
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeAzure, FreeAPIKey: "azure-key-check-bad"}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err == nil {
			t.Fatal("接口 403 时 CheckTranslateModel err = nil， want 错误")
		}
	})

	t.Run("谷歌接口就绪", func(t *testing.T) {
		stub := newGoogleStub(t, "google-key-check-ok", "你好", http.StatusOK)
		t.Cleanup(stub.Close)
		useGoogleBase(t, stub.URL)
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeGoogle, FreeAPIKey: "google-key-check-ok"}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err != nil {
			t.Fatalf("CheckTranslateModel err = %v， want nil", err)
		}
	})

	t.Run("Cloudflare 接口就绪", func(t *testing.T) {
		stub := newCFStub(t, "acct-check-ok", "你好", http.StatusOK, true)
		t.Cleanup(stub.Close)
		useCFBase(t, stub.URL)
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Provider: ProviderFree, FreeProvider: FreeCloudflare, CFAccountID: "acct-check-ok", CFToken: "tok-abc"}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err != nil {
			t.Fatalf("CheckTranslateModel err = %v， want nil", err)
		}
	})

	t.Run("翻译已关闭", func(t *testing.T) {
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: false, Provider: ProviderOff}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err == nil {
			t.Fatal("关闭时 CheckTranslateModel err = nil， want 错误")
		}
	})
}

// 直播字幕的翻译器：关闭时返回 nil（管线直接出英文原文），免费接口时要有翻译器。
func TestTranslatorForNilWhenOff(t *testing.T) {
	svc, _ := newTranslateService(t)
	if got := svc.translatorFor(TranslateSettings{Provider: ProviderOff}); got != nil {
		t.Fatalf("关闭时 translatorFor = %v， want nil", got)
	}
	if got := svc.translatorFor(TranslateSettings{Provider: ProviderFree, FreeProvider: FreeAzure}); got == nil {
		t.Fatal("免费接口时 translatorFor = nil， want 翻译器")
	}
}
