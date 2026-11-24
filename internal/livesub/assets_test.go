// Package livesub 的资源下载测试：whisper 程序与模型首次使用下载缓存。
package livesub

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/unbox/unbox/internal/httpx"
)

func TestEnsureAssets_DownloadsAndCaches(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("仅 Windows/Linux 有预编译 whisper 程序")
	}
	// 造一个最小 zip：Release/whisper-server(.exe) + 一个 DLL。
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	binName := "whisper-server"
	if runtime.GOOS == "windows" {
		binName = "whisper-server.exe"
	}
	w, _ := zw.Create("Release/" + binName)
	_, _ = w.Write([]byte("fake-server"))
	w, _ = zw.Create("Release/ggml.dll")
	_, _ = w.Write([]byte("fake-dll"))
	_ = zw.Close()

	modelData := []byte("fake-model-bytes")
	var binHits, modelHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin.zip":
			binHits++
			w.Header().Set("Content-Length", strconv.Itoa(zipBuf.Len()))
			_, _ = w.Write(zipBuf.Bytes())
		case "/model.bin":
			modelHits++
			_, _ = w.Write(modelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	// 用测试服务器覆盖默认下载地址。
	t.Setenv("UNBOX_LIVESUB_BIN_URL", srv.URL+"/bin.zip")
	t.Setenv("UNBOX_LIVESUB_MODEL_URL", srv.URL+"/model.bin")

	binPath, modelPath, err := EnsureAssets(t.Context(), dir, "base.en", nil)
	if err != nil {
		t.Fatalf("首次下载失败: %v", err)
	}
	if got, err := os.ReadFile(binPath); err != nil || string(got) != "fake-server" {
		t.Fatalf("程序文件不对: %v %q", err, got)
	}
	if got, err := os.ReadFile(modelPath); err != nil || string(got) != "fake-model-bytes" {
		t.Fatalf("模型文件不对: %v %q", err, got)
	}
	// 目录里应有 DLL（exe 没 DLL 起不来）。
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(filepath.Join(filepath.Dir(binPath), "ggml.dll")); err != nil {
			t.Fatalf("DLL 没随程序一起解开: %v", err)
		}
	}
	if binHits != 1 || modelHits != 1 {
		t.Fatalf("下载次数 = 程序 %d、模型 %d，想得到各 1 次", binHits, modelHits)
	}

	// 第二次应直接用缓存，不再下载。
	_, _, err = EnsureAssets(t.Context(), dir, "base.en", nil)
	if err != nil {
		t.Fatalf("用缓存失败: %v", err)
	}
	if binHits != 1 || modelHits != 1 {
		t.Fatalf("用缓存时又下载了：程序 %d、模型 %d", binHits, modelHits)
	}
}

func TestEnsureAssets_BadModel(t *testing.T) {
	if _, _, err := EnsureAssets(t.Context(), t.TempDir(), "not-a-model", nil); err == nil {
		t.Fatal("非法模型名应返回错误")
	}
}

func TestEnsureAssets_DownloadFallback(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("仅 Windows/Linux 有预编译 whisper 程序")
	}
	// 第一个地址 404，第二个才有内容：应自动换备用地址。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.zip" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	// 直接测底层下载：坏地址在前、好地址在后。
	if err := downloadFile(t.Context(), dir, "f.bin", []string{srv.URL + "/bad.zip", srv.URL + "/good.bin"}, nil); err != nil {
		t.Fatalf("备用地址应能下载成功: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.bin")); err != nil {
		t.Fatalf("下载的文件不存在: %v", err)
	}
}

func TestEnsureAssets_AllURLsFail(t *testing.T) {
	dir := t.TempDir()
	if err := downloadFile(t.Context(), dir, "f.bin", []string{"http://127.0.0.1:1/nope"}, nil); err == nil {
		t.Fatal("全部地址失败时应返回错误")
	}
}

// TestDownloadClientUsesEnvProxy：模型/程序下载走外网，必须经统一代理选择，
// 否则只改系统代理（v2rayN）的桌面启动方式下下载直连慢路。
// Linux 上裸客户端的 DefaultTransport 本来就认环境变量，行为测不出差别，
// 真正的差别在 Windows 系统代理，所以这里钉接线：传输层的 Proxy 必须是
// 统一选择函数 httpx.ProxyForRequest。
func TestDownloadClientUsesEnvProxy(t *testing.T) {
	tr, ok := downloadClient.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("下载客户端没有显式传输层（Transport = %v）", downloadClient.Transport)
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(httpx.ProxyForRequest).Pointer() {
		t.Fatalf("下载客户端没走统一代理选择函数 httpx.ProxyForRequest")
	}

	// 行为烟囱：经环境变量代理下载到一个直连 DNS 失败的地址。
	var viaProxy atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Add(1)
		_, _ = w.Write([]byte("fake-model-bytes"))
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	dest := filepath.Join(t.TempDir(), "f.bin")
	if err := downloadOne(context.Background(), dest, "http://model-test.invalid/f.bin", nil); err != nil {
		t.Fatalf("downloadOne 报错: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "fake-model-bytes" {
		t.Fatalf("下载内容 = %q / %v，想要 fake-model-bytes", got, err)
	}
	if viaProxy.Load() != 1 {
		t.Fatalf("经代理的请求次数 = %d，想要 1", viaProxy.Load())
	}
}
