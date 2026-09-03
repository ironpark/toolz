package cmd

import (
	"context"
	"encoding/json"
	"fmt"
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
			&cli.BoolFlag{Name: "hook", Usage: "hook socket 우선, 실패 시 polling 폴백"},
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
	if x.cmd.Bool("hook") {
		// hook 은 최적화이고 polling 이 기본이다. hook 경로가 없어도 감지는
		// 정상 동작해야 하므로, 미구현을 오류로 만들지 않고 폴백한다 (§6.1).
		fmt.Fprintln(x.stderr, "hook 경로가 아직 없습니다. polling 으로 진행합니다")
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

// hookCommand — git hook 과 에이전트 도구 hook 을 관리한다 (§6).
func hookCommand() *cli.Command {
	targetFlags := func() []cli.Flag {
		return []cli.Flag{
			&cli.BoolFlag{Name: "git", Usage: "reference-transaction hook"},
			&cli.BoolFlag{Name: "agent-tools", Usage: "감지된 에이전트 도구 전부"},
		}
	}
	return &cli.Command{
		Name:  "hook",
		Usage: "hook 을 설치·제거·점검한다",
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "hook 을 설치한다",
				Flags: append(targetFlags(),
					&cli.BoolFlag{Name: "claude-code", Usage: "SessionStart/SessionEnd 를 .claude/settings.json 에"},
					&cli.BoolFlag{Name: "codex", Usage: "SessionStart/SessionEnd 를 .codex/hooks.json 에"},
					&cli.BoolFlag{Name: "force", Usage: "충돌하는 기존 설정을 덮어씀"},
				),
				Action: func(_ context.Context, _ *cli.Command) error {
					return notImplemented("hook install")
				},
			},
			{
				Name:  "uninstall",
				Usage: "hook 을 제거한다",
				Flags: targetFlags(),
				Action: func(_ context.Context, _ *cli.Command) error {
					return notImplemented("hook uninstall")
				},
			},
			{
				Name:  "status",
				Usage: "hook 설치 상태를 출력한다",
				Action: func(_ context.Context, _ *cli.Command) error {
					return notImplemented("hook status")
				},
			},
		},
	}
}
