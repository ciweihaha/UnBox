package library

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerServesRegisteredFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(p, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newServer()
	defer s.close()
	u, err := s.register(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "http://127.0.0.1:") {
		t.Fatalf("register url=%q", u)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "fake-video-bytes" {
		t.Fatalf("body=%q", b)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("CORS 头=%q, want *", got)
	}
}

func TestServerRegisterDeduplicatesPath(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "a.mp4")
	u1, err := s.register(p)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := s.register(p)
	if err != nil {
		t.Fatal(err)
	}
	if u1 != u2 {
		t.Fatalf("同一路径应复用注册: %q != %q", u1, u2)
	}
}

// TestServerRegisterAfterCloseReturnsError 是回归测试（第 36 轮）：监听
// 起不来/服务已关闭时 register 不得返回无端口的死 URL
// （http://127.0.0.1/v/...），调用方无从得知；应报错。
func TestServerRegisterAfterCloseReturnsError(t *testing.T) {
	s := newServer()
	p := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.register(p); err != nil {
		t.Fatalf("首次注册应成功: %v", err)
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	// 已登记过的路径也不得再给死 URL（去重分支同样要守卫）。
	if u, err := s.register(p); err == nil {
		t.Fatalf("关闭后注册应报错，实际返回 %q", u)
	}
	if u, err := s.register(filepath.Join(t.TempDir(), "b.mp4")); err == nil {
		t.Fatalf("关闭后注册新路径应报错，实际返回 %q", u)
	}
}

func TestServerRejectsBadToken(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, err := s.register(p)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(u, "?t=", "?t=wrong", 1)
	resp, err := http.Get(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token should be 403, got %d", resp.StatusCode)
	}
}

func TestServerUsesOpaqueIDAndSupportsRange(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "private", "a.mp4")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, err := s.register(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u, p) || strings.Contains(u, filepath.Dir(p)) {
		t.Fatalf("file path leaked in URL %q", u)
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(b) != "2345" {
		t.Fatalf("range response status=%d body=%q", resp.StatusCode, b)
	}
}

func TestServerUnknownIDAndClose(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, err := s.register(p)
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.Replace(u, "/v/1?", "/v/999?", 1)
	resp, err := http.Get(unknown)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id should be 404, got %d", resp.StatusCode)
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
}
