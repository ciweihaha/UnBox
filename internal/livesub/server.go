package livesub

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// execWhisperServer 是 WhisperServer 的真实实现：
// 拉起 whisper-server 子进程（常驻，模型只加载一次），
// 停时杀掉进程。端口用空闲端口，不与播放代理抢。
type execWhisperServer struct {
	cmd      *exec.Cmd
	endpoint string
}

// Start 拉起 whisper-server：-m 模型 --host 127.0.0.1 --port 空闲端口
// --language en（直播是英文，固定语言省掉检测）。
// 模型 148MB 首次加载要几秒，等就绪（最多 60 秒）才返回。
func (s *execWhisperServer) Start(ctx context.Context, binPath, modelPath string) (string, error) {
	port, err := freePort()
	if err != nil {
		return "", fmt.Errorf("分配语音识别端口失败: %w", err)
	}
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	cmd := exec.Command(binPath,
		"-m", modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--language", "en",
	)
	// DLL 与 exe 同目录：工作目录设到程序目录，Windows 上找 DLL 最稳。
	cmd.Dir = filepath.Dir(binPath)
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动语音识别服务失败: %w", err)
	}
	s.cmd = cmd
	s.endpoint = endpoint
	client := NewWhisperClient(endpoint, 2*time.Second)
	if err := client.WaitReady(ctx, 60*time.Second); err != nil {
		_ = s.Stop()
		return "", fmt.Errorf("语音识别服务未就绪: %w", err)
	}
	return endpoint, nil
}

// Stop 杀掉子进程并收尸。
func (s *execWhisperServer) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.cmd = nil
	return nil
}

// freePort 找一个空闲本地端口。
func freePort() (int, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
