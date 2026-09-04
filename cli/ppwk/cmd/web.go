package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/web"
	"github.com/urfave/cli/v3"
)

// webCommand — 보드를 브라우저에서 본다 (§6).
func webCommand() *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "보드를 브라우저에서 연다",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "addr",
				Usage: "듣는 주소. 기본은 루프백의 빈 포트",
				Value: "127.0.0.1:0",
			},
			&cli.BoolFlag{
				Name:  "no-open",
				Usage: "브라우저를 자동으로 열지 않음",
			},
			&cli.DurationFlag{
				Name:  "poll",
				Usage: "변경 감지 주기",
				Value: time.Second,
			},
		},
		// watch 와 같은 이유로 ctx 를 받는다 — 수명이 긴 명령이고 SIGINT 는
		// 오류가 아니다.
		Action: func(ctx context.Context, c *cli.Command) error {
			return classify(runWeb(ctx, newCtx(c)))
		},
	}
}

func runWeb(ctx context.Context, x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	if ok, err := b.Initialized(); err != nil {
		return err
	} else if !ok {
		return UsageError("보드가 초기화되지 않았습니다. ppwk init 을 먼저 실행하세요")
	}

	addr := x.cmd.String("addr")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	url := "http://" + listener.Addr().String()
	if !web.Loopback(addr) {
		// 인증이 없다. 루프백 밖에 묶는 것은 같은 네트워크의 누구에게나
		// 보드를 열어 주는 것과 같으므로, 조용히 지나가지 않는다.
		fmt.Fprintf(x.stderr, "경고: %s 는 루프백이 아닙니다. 이 주소에 닿는 누구나 보드를 바꿀 수 있습니다\n", addr)
	}
	x.printf("%s\n", url)

	server := &http.Server{
		Handler: web.New(b, web.Options{PollInterval: x.cmd.Duration("poll")}),
		// SSE 가 계속 열려 있으므로 쓰기 시한을 두지 않는다. 읽기 헤더에만
		// 시한을 둬서 연결만 잡고 있는 상대를 막는다.
		ReadHeaderTimeout: 10 * time.Second,
	}

	if !x.cmd.Bool("no-open") {
		openBrowser(url)
	}

	// SIGINT 로 끝낸다. 종료는 오류가 아니다.
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-signalCtx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
		return nil
	}
}

// openBrowser 는 기본 브라우저를 연다. 실패는 조용히 넘긴다.
//
// 주소는 이미 출력했다. 브라우저를 못 여는 것은 (헤드리스 서버, WSL 등)
// 정상적인 상황이지 오류가 아니다.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
