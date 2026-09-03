package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/watch"
	"github.com/urfave/cli/v3"
)

// watchCommand — ref 변경을 스트림으로 내보낸다 (§6).
// polling 이 기본이고 hook 은 최적화다. 첫 실행은 베이스라인만 잡는다.
func watchCommand() *cli.Command {
	return &cli.Command{
		Name:  "watch",
		Usage: "ref 변경을 감지해 줄당 JSON 으로 내보낸다",
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Name:    "interval",
				Value:   2 * time.Second,
				Usage:   "polling 주기",
				Sources: cli.EnvVars("PPWK_POLL_INTERVAL"),
			},
			&cli.StringFlag{Name: "filter", Usage: "특정 ref prefix 만"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return classify(runWatch(ctx, newCtx(c)))
		},
	}
}

// runWatch 는 변경을 줄당 JSON 으로 흘려보낸다 (§6).
//
// SIGINT 는 오류가 아니라 정상 종료다. 그래서 action() 대신 ctx 를 받는
// 형태를 쓴다 — 이 명령만 유일하게 수명이 긴 명령이다.
func runWatch(ctx context.Context, x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	enc := json.NewEncoder(x.stdout)
	return b.Watch(signalCtx, board.WatchOptions{
		Interval: x.cmd.Duration("interval"),
		Filter:   x.cmd.String("filter"),
	}, func(event watch.Event) error {
		if x.quiet {
			return nil
		}
		return enc.Encode(event)
	})
}

// hookCommand — 코딩 에이전트 도구의 세션 훅을 관리한다 (§6, §3.8 층 3).
//
// git 의 reference-transaction 훅은 두지 않는다. 그것이 사 오는 것은 알림
// 지연 1~2초를 없애는 것뿐인데, socat 의존과 공용 hooks 디렉터리 설치,
// socket 수명 관리, 그리고 잘못하면 저장소의 모든 ref 쓰기가 멈추는 실패
// 모드를 함께 사 와야 한다. polling 이 기본이라는 §6.1 의 결론을 그대로
// 따른다.
func hookCommand() *cli.Command {
	targetFlags := func() []cli.Flag {
		return []cli.Flag{
			&cli.BoolFlag{Name: "claude-code", Usage: "SessionStart/SessionEnd 를 .claude/settings.json 에"},
			&cli.BoolFlag{Name: "codex", Usage: "SessionStart/SessionEnd 를 .codex/hooks.json 에"},
			&cli.BoolFlag{Name: "agent-tools", Usage: "지원하는 도구 전부"},
		}
	}
	return &cli.Command{
		Name:  "hook",
		Usage: "도구 세션 훅을 설치·제거·점검한다",
		Commands: []*cli.Command{
			{
				Name:   "install",
				Usage:  "훅을 설치한다",
				Flags:  append(targetFlags(), &cli.BoolFlag{Name: "force", Usage: "충돌하는 기존 ppwk 설정을 덮어씀"}),
				Action: action(runHookInstall),
			},
			{
				Name:   "uninstall",
				Usage:  "ppwk 가 등록한 훅만 제거한다",
				Flags:  targetFlags(),
				Action: action(runHookUninstall),
			},
			{
				Name:   "status",
				Usage:  "훅 설치 상태를 출력한다",
				Action: action(runHookStatus),
			},
		},
	}
}
