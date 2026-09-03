package cmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

// nextCommand — 에이전트가 호출하는 유일한 스케줄링 명령 (§4).
// 후보가 없으면 오류가 아니라 exit 0 에 빈 결과다.
func nextCommand() *cli.Command {
	return &cli.Command{
		Name:  "next",
		Usage: "다음에 할 이슈를 고른다",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "claim", Usage: "후보를 claim 까지 수행"},
			&cli.StringFlag{Name: "plan", Usage: "특정 plan 으로 제한"},
			&cli.StringFlag{Name: "label", Usage: "capability 필터"},
			&cli.BoolFlag{Name: "dry-run", Usage: "후보 목록만 표시. 저장소를 변형하지 않음"},
			&cli.IntFlag{Name: "max-attempts", Value: 5, Usage: "claim 시도 상한"},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("next")
		},
	}
}

// reapCommand — 죽은 소유자가 붙잡은 이슈를 open 으로 되돌린다 (§4).
// 평소에는 next 가 자동으로 수행한다.
func reapCommand() *cli.Command {
	return &cli.Command{
		Name:  "reap",
		Usage: "죽은 소유자의 이슈를 회수한다",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Usage: "회수 대상만 표시"},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("reap")
		},
	}
}

// agentsCommand — 잠금 파일을 읽어 에이전트 현황을 출력한다 (§4, D13).
func agentsCommand() *cli.Command {
	return &cli.Command{
		Name:  "agents",
		Usage: "에이전트 잠금 현황을 출력한다",
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("agents")
		},
	}
}

// internalCommand — 도구 훅 전용. --help 에 노출하지 않는다 (§4).
func internalCommand() *cli.Command {
	return &cli.Command{
		Name:   "internal",
		Hidden: true,
		Commands: []*cli.Command{
			{
				Name:  "session-event",
				Usage: "stdin 의 훅 JSON 을 처리한다",
				// 훅에서 실행되므로 알 수 없는 입력이나 오류에는 조용히 exit 0 한다.
				Action: func(_ context.Context, _ *cli.Command) error {
					return notImplemented("internal session-event")
				},
			},
		},
	}
}
