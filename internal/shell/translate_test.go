package shell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// newOllamaStub 模拟本地 Ollama：/api/chat 返回固定译文，/api/tags 返回固定模型列表。
func newOllamaStub(t *testing.T, translated string, chatStatus int, models []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			w.WriteHeader(chatStatus)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]string{"content": translated},
			})
		case "/api/tags":
			entries := make([]map[string]string, 0, len(models))
			for _, m := range models {
				entries = append(entries, map[string]string{"name": m})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": entries})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTranslateSettingsDefaults(t *testing.T) {
	svc, _ := newTranslateService(t)
	got := svc.GetTranslateSettings()
	if got.Enabled {
		t.Fatalf("默认 Enabled = true， want false（翻译默认关）")
	}
	if got.Model != "qwen2.5:3b" {
		t.Fatalf("默认 Model = %q， want qwen2.5:3b", got.Model)
	}
	if got.Endpoint != "http://127.0.0.1:11434" {
		t.Fatalf("默认 Endpoint = %q， want http://127.0.0.1:11434", got.Endpoint)
	}
}

func TestTranslateSettingsRoundTrip(t *testing.T) {
	svc, st := newTranslateService(t)
	want := TranslateSettings{Enabled: true, Model: "qwen2.5:7b", Endpoint: "http://127.0.0.1:11434"}
	if err := svc.SetTranslateSettings(want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetTranslateSettings(); got != want {
		t.Fatalf("round trip = %#v， want %#v", got, want)
	}
}

func TestTranslateSettingsEmptyValuesFallBackToDefaults(t *testing.T) {
	svc, st := newTranslateService(t)
	if err := st.SetKV("translate.model", "   "); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKV("translate.endpoint", ""); err != nil {
		t.Fatal(err)
	}
	got := svc.GetTranslateSettings()
	if got.Model != "qwen2.5:3b" || got.Endpoint != "http://127.0.0.1:11434" {
		t.Fatalf("空值回退 = %#v， want 默认模型与地址", got)
	}
}

func TestTranslateSubtitleDisabledReturnsOriginalWithoutRequest(t *testing.T) {
	stub := newOllamaStub(t, "你好", http.StatusOK, []string{"qwen2.5:3b"})
	t.Cleanup(stub.Close)
	svc, _ := newTranslateService(t)
	// 开关默认关闭：即使地址指向可用的 stub，也必须不发请求、直接返回原文。
	// 若实现误发请求，stub 会返回译文导致下面断言失败。
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: false, Endpoint: stub.URL}); err != nil {
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

func TestTranslateSubtitleEnabledTranslates(t *testing.T) {
	stub := newOllamaStub(t, "你好，世界", http.StatusOK, []string{"qwen2.5:3b"})
	t.Cleanup(stub.Close)
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Endpoint: stub.URL}); err != nil {
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

func TestTranslateSubtitleServerErrorReturnsError(t *testing.T) {
	stub := newOllamaStub(t, "", http.StatusInternalServerError, nil)
	t.Cleanup(stub.Close)
	svc, _ := newTranslateService(t)
	if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Endpoint: stub.URL}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TranslateSubtitle("Hello"); err == nil {
		t.Fatal("服务端 500 时 TranslateSubtitle err = nil， want 错误（调用方据此回退原文）")
	}
}

func TestCheckTranslateModel(t *testing.T) {
	t.Run("模型已拉取", func(t *testing.T) {
		stub := newOllamaStub(t, "", http.StatusOK, []string{"qwen2.5:3b"})
		t.Cleanup(stub.Close)
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Endpoint: stub.URL}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err != nil {
			t.Fatalf("CheckTranslateModel err = %v， want nil", err)
		}
	})

	t.Run("模型未拉取", func(t *testing.T) {
		stub := newOllamaStub(t, "", http.StatusOK, []string{"llama3.1:8b"})
		t.Cleanup(stub.Close)
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Endpoint: stub.URL}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err == nil {
			t.Fatal("模型不在列表里时 CheckTranslateModel err = nil， want 错误")
		} else if !strings.Contains(err.Error(), "qwen2.5:3b") {
			t.Fatalf("CheckTranslateModel err = %v， want 提示缺失的模型名", err)
		}
	})

	t.Run("服务没起", func(t *testing.T) {
		stub := newOllamaStub(t, "", http.StatusOK, nil)
		deadURL := stub.URL
		stub.Close()
		svc, _ := newTranslateService(t)
		if err := svc.SetTranslateSettings(TranslateSettings{Enabled: true, Endpoint: deadURL}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckTranslateModel(); err == nil {
			t.Fatal("服务没起时 CheckTranslateModel err = nil， want 错误")
		}
	})
}
