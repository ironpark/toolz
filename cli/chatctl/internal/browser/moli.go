package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// moliServer는 백그라운드로 띄운 `moli serve` 프로세스입니다.
type moliServer struct {
	// url은 chromedp 가 접속할 CDP 주소입니다.
	url  string
	stop func()
}

// startMoli는 빈 포트에 moli serve 를 띄우고 CDP 접속 주소를 돌려줍니다.
func startMoli(ctx context.Context, bin, profileDir string) (*moliServer, error) {
	if bin == "" {
		return nil, fmt.Errorf("moli 를 찾을 수 없습니다 (https://github.com/lexmount/moli). --engine chrome 을 사용하세요")
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("moli 프로필 디렉터리 생성 실패: %w", err)
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}

	// 컨텍스트가 끝나면 프로세스도 함께 정리되도록 CommandContext 를 씁니다.
	cmd := exec.CommandContext(ctx, bin, "serve",
		"--host", "127.0.0.1",
		"--port", fmt.Sprint(port),
		"--profile-dir", profileDir,
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("moli serve 실행 실패: %w", err)
	}

	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if err := waitForMoli(ctx, url+"json/version", 10*time.Second); err != nil {
		stop()
		return nil, err
	}
	// chromedp 가 /json/version 에서 웹소켓 주소를 알아서 찾아냅니다.
	return &moliServer{url: url, stop: stop}, nil
}

// waitForMoli는 CDP 엔드포인트가 응답할 때까지 기다립니다.
func waitForMoli(ctx context.Context, endpoint string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{Timeout: time.Second}
	for {
		resp, err := client.Get(endpoint)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("moli serve 가 %s 안에 준비되지 않았습니다", timeout)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// freePort는 비어 있는 로컬 포트를 하나 고릅니다.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("빈 포트를 찾지 못했습니다: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
