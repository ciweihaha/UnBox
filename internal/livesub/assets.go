package livesub

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/httpx"
)

// whisperRelease 是预编译 whisper.cpp 程序的版本（b5454，2026-10-06）：
// 带 whisper-server.exe 的 whisper-bin-x64.zip，约 8.5MB。
const whisperRelease = "b5454"

// downloadTimeout 是单个文件下载超时：模型 148MB，
// 云电脑直连 GitHub 慢，放宽到 30 分钟；hf-mirror 正常几分钟下完。
const downloadTimeout = 30 * time.Minute

// downloadClient 是模型/程序下载客户端：下载是长耗时流，
// 不能设 http.Client 整体 Timeout（会把下载中途掐断），
// 只给传输层配代理选择 + 30 秒 TLS 握手上限，总时限由调用方的 ctx 管。
// 代理走 httpx（环境变量优先，其次 Windows 系统代理），
// 不能用 http.DefaultClient 直连——只改系统代理的桌面启动方式下直连慢路。
var downloadClient = &http.Client{Transport: httpx.NewTransport(30 * time.Second)}

// 模型文件名：base.en 准确、tiny.en 更快；
// 只支持这两个，别的模型名直接报错，别让用户下错文件。
var modelFiles = map[string]string{
	"base.en": "ggml-base.en.bin",
	"tiny.en": "ggml-tiny.en.bin",
}

// ProgressFunc 下载进度回调（已下载字节、总字节；总字节未知时为 -1）。
type ProgressFunc func(downloaded, total int64)

// EnsureAssets 保证 whisper 程序与模型都在本地：已缓存直接用，
// 缺失才下载（程序约 8.5MB、模型 75–148MB，首次使用下载一次，
// 之后缓存复用，绝不打进安装包）。返回程序与模型路径。
func EnsureAssets(ctx context.Context, dir, model string, progress ProgressFunc) (binPath, modelPath string, err error) {
	modelFile, ok := modelFiles[model]
	if !ok {
		return "", "", fmt.Errorf("不支持的语音识别模型 %q（只支持 base.en、tiny.en）", model)
	}
	binDir := filepath.Join(dir, "bin")
	modelDir := filepath.Join(dir, "models")
	binName := "whisper-server"
	if runtime.GOOS == "windows" {
		binName = "whisper-server.exe"
	}
	binPath = filepath.Join(binDir, binName)
	modelPath = filepath.Join(modelDir, modelFile)

	if _, err := os.Stat(binPath); err != nil {
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return "", "", fmt.Errorf("创建语音识别目录失败: %w", err)
		}
		if err := downloadBinary(ctx, binDir, progress); err != nil {
			return "", "", err
		}
	}
	if _, err := os.Stat(modelPath); err != nil {
		if err := os.MkdirAll(modelDir, 0o755); err != nil {
			return "", "", fmt.Errorf("创建模型目录失败: %w", err)
		}
		if err := downloadModel(ctx, modelPath, modelFile, progress); err != nil {
			return "", "", err
		}
	}
	return binPath, modelPath, nil
}

// downloadBinary 下载并解开 whisper 程序压缩包。
func downloadBinary(ctx context.Context, binDir string, progress ProgressFunc) error {
	urls := binaryURLs()
	archive := filepath.Join(binDir, "whisper-pkg.tmp")
	defer os.Remove(archive)
	if err := downloadFile(ctx, binDir, "whisper-pkg.tmp", urls, progress); err != nil {
		return fmt.Errorf("下载语音识别程序失败: %w", err)
	}
	if strings.HasSuffix(urls[0], ".zip") || strings.Contains(archive, ".zip") {
		if err := extractZip(archive, binDir); err != nil {
			return fmt.Errorf("解开语音识别程序失败: %w", err)
		}
		return nil
	}
	if err := extractTarGz(archive, binDir); err != nil {
		return fmt.Errorf("解开语音识别程序失败: %w", err)
	}
	return nil
}

// binaryURLs 返回程序压缩包候选地址（主 + 备用镜像）。
// 测试可用 UNBOX_LIVESUB_BIN_URL 覆盖（单个地址）。
func binaryURLs() []string {
	if v := strings.TrimSpace(os.Getenv("UNBOX_LIVESUB_BIN_URL")); v != "" {
		return []string{v}
	}
	base := "https://github.com/ggml-org/whisper.cpp/releases/download/" + whisperRelease + "/"
	switch runtime.GOOS {
	case "windows":
		return []string{
			base + "whisper-bin-x64.zip",
			"https://ghfast.top/" + base + "whisper-bin-x64.zip",
		}
	case "linux":
		return []string{
			base + "whisper-bin-ubuntu-x64.tar.gz",
			"https://ghfast.top/" + base + "whisper-bin-ubuntu-x64.tar.gz",
		}
	default:
		return nil
	}
}

// downloadModel 下载模型文件（hf-mirror 优先，国内快；失败回退官方）。
// 测试可用 UNBOX_LIVESUB_MODEL_URL 覆盖（单个地址）。
func downloadModel(ctx context.Context, dest, modelFile string, progress ProgressFunc) error {
	urls := []string{
		"https://hf-mirror.com/ggerganov/whisper.cpp/resolve/main/" + modelFile,
		"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile,
	}
	if v := strings.TrimSpace(os.Getenv("UNBOX_LIVESUB_MODEL_URL")); v != "" {
		urls = []string{v}
	}
	if err := downloadFile(ctx, filepath.Dir(dest), filepath.Base(dest)+".tmp", urls, progress); err != nil {
		return fmt.Errorf("下载语音识别模型 %s 失败: %w", modelFile, err)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(dest), filepath.Base(dest)+".tmp"), dest); err != nil {
		return fmt.Errorf("保存语音识别模型失败: %w", err)
	}
	return nil
}

// downloadFile 按顺序试候选地址，第一个成功的写入 dir/name（临时名）；
// 全部失败返回最后一个错误。progress 可为空。
func downloadFile(ctx context.Context, dir, name string, urls []string, progress ProgressFunc) error {
	if len(urls) == 0 {
		return fmt.Errorf("没有可用的下载地址（当前系统 %s 暂无预编译程序）", runtime.GOOS)
	}
	var lastErr error
	for _, u := range urls {
		if err := downloadOne(ctx, filepath.Join(dir, name), u, progress); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func downloadOne(ctx context.Context, dest, url string, progress ProgressFunc) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("下载 %s 返回 HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	var downloaded int64
	buf := make([]byte, 128<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, resp.ContentLength)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// extractZip 解 zip 包：只取常规文件，按 basename 平铺到 binDir
// （whisper-bin-x64.zip 里程序与 DLL 都在 Release/ 下，
// exe 没 DLL 起不来，必须整包解开，不能只抽 exe）。
func extractZip(archive, binDir string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if name == "." || name == "/" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(binDir, name), rc); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

// extractTarGz 解 tar.gz 包：同上，只取常规文件按 basename 平铺。
func extractTarGz(archive, binDir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if name == "." || name == "/" {
			continue
		}
		if err := writeFile(filepath.Join(binDir, name), tr); err != nil {
			return err
		}
	}
}

func writeFile(dest string, r io.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}
