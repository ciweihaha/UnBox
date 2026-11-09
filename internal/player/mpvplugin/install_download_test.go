package mpvplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stallTransport 返回一个「建连成功、此后永远不吐字节」的响应体，模拟下载中途
// 停滞（服务器挂死、中间设备黑洞）。Read 只在请求 ctx 取消时返回，避免测试本身卡死。
type stallTransport struct{}

func (stallTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&stallReader{ctx: req.Context()}),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type stallReader struct{ ctx context.Context }

func (s *stallReader) Read([]byte) (int, error) {
	<-s.ctx.Done()
	return 0, s.ctx.Err()
}

// TestInstallWindowsStallTimesOut 下载停滞时安装不应无限挂起：超过下载时限后
// 必须返回超时错误。调用方传的是 context.Background()，时限只能由本函数兜底。
func TestInstallWindowsStallTimesOut(t *testing.T) {
	m := newManager("windows", t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
	m.client = &http.Client{Transport: stallTransport{}}

	oldTimeout := windowsDownloadTimeout
	windowsDownloadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { windowsDownloadTimeout = oldTimeout })

	done := make(chan error, 1)
	go func() {
		_, err := m.Install(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "超时") {
			t.Fatalf("停滞下载应报超时错误，got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("下载停滞时 Install 应在超时后返回，而不是无限挂起")
	}
}

// fakeInstallerTransport 返回固定内容供安装流程走完下载 + 校验；测试把
// windowsSHA 换成该内容的哈希。
type fakeInstallerTransport struct{ body string }

func (f fakeInstallerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(f.body)),
		Header:        make(http.Header),
		ContentLength: int64(len(f.body)),
		Request:       req,
	}, nil
}

func installWindowsTestManager(t *testing.T, body string) *Manager {
	t.Helper()
	m := newManager("windows", t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
	m.client = &http.Client{Transport: fakeInstallerTransport{body: body}}
	sum := sha256.Sum256([]byte(body))
	oldSHA := windowsSHA
	windowsSHA = hex.EncodeToString(sum[:])
	t.Cleanup(func() { windowsSHA = oldSHA })
	return m
}

// TestInstallWindowsVerifiesInstalledBinary 安装程序退出码为 0 不代表 mpv 真
// 落盘（杀软隔离、磁盘满都可能让安装「成功」却无文件）；事后必须按插件路径
// 校验文件存在，缺失即报错，而不是报「安装完成」。
func TestInstallWindowsVerifiesInstalledBinary(t *testing.T) {
	m := installWindowsTestManager(t, "fake-installer-bytes")
	m.run = func(context.Context, string, ...string) error { return nil } // 安装「成功」，但什么都没写
	_, err := m.Install(context.Background())
	if err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("安装后无 mpv 文件应报错，got %v", err)
	}
}

// TestInstallWindowsSucceedsWhenBinaryPresent 安装程序正常落盘时仍应报安装完成。
func TestInstallWindowsSucceedsWhenBinaryPresent(t *testing.T) {
	m := installWindowsTestManager(t, "fake-installer-bytes")
	m.run = func(_ context.Context, _ string, args ...string) error {
		for _, a := range args {
			if dir, ok := strings.CutPrefix(a, "/DIR="); ok {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "mpv.exe"), []byte("fake"), 0o755)
			}
		}
		return errors.New("安装命令缺少 /DIR=")
	}
	res, err := m.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Installed {
		t.Fatalf("Install = %+v, want Installed", res)
	}
}
