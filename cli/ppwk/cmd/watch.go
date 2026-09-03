package cmd

import (
	"context"
	"time"

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
				Sources: cli.EnvVars("PAPERWORK_POLL_INTERVAL"),
			},
			&cli.BoolFlag{Name: "hook", Usage: "hook socket 우선, 실패 시 polling 폴백"},
			&cli.StringFlag{Name: "filter", Usage: "특정 ref prefix 만"},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("watch")
		},
	}
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
