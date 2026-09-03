package cmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

// addCommand — 이슈를 생성한다 (§2).
func addCommand() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "이슈를 생성한다",
		ArgsUsage: "<title>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "priority", Value: "med", Usage: "high|med|low|none. none 은 백로그"},
			&cli.StringSliceFlag{Name: "label", Usage: "라벨. 반복 가능"},
			&cli.StringSliceFlag{Name: "depends-on", Usage: "선행 이슈 ID. 반복 가능"},
			&cli.StringFlag{Name: "body", Usage: "본문"},
			&cli.StringFlag{Name: "body-file", Usage: "본문 파일 경로"},
			&cli.BoolFlag{Name: "body-stdin", Usage: "본문을 stdin 에서 읽음"},
			&cli.StringFlag{Name: "plan", Usage: "소속 plan ID"},
			&cli.StringFlag{Name: "phase", Usage: "소속 phase ID"},
			&cli.IntFlag{Name: "seq", Usage: "phase 내 순번. 생략 시 최대값 + 10"},
		},
		Action: action(runAdd),
	}
}

// listCommand — 이슈 목록을 출력한다 (§2).
func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "이슈 목록을 출력한다",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{Name: "status", Usage: "상태 필터. 반복 가능"},
			&cli.StringSliceFlag{Name: "priority", Usage: "우선순위 필터. 반복 가능"},
			&cli.StringFlag{Name: "owner", Usage: "소유 에이전트"},
			&cli.StringFlag{Name: "label", Usage: "라벨"},
			&cli.StringFlag{Name: "plan", Usage: "plan ID"},
			&cli.StringFlag{Name: "phase", Usage: "phase ID"},
			&cli.BoolFlag{Name: "unassigned", Usage: "소유자 없는 것만"},
			&cli.BoolFlag{Name: "mine", Usage: "현재 세션이 claim 한 것만"},
			&cli.BoolFlag{Name: "archived", Usage: "archive 만"},
			&cli.BoolFlag{Name: "all", Usage: "issues + archive"},
			&cli.StringFlag{Name: "sort", Usage: "next|id|updated|priority"},
			&cli.IntFlag{Name: "limit", Usage: "출력 개수 상한"},
		},
		Action: action(runList),
	}
}

// showCommand — 이슈 전체를 출력한다. archive 도 찾는다 (§2).
func showCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "이슈 하나를 출력한다",
		ArgsUsage: "<id>",
		Action:    action(runShow),
	}
}

// historyCommand — commit chain 을 이벤트 순서로 출력한다 (§2).
func historyCommand() *cli.Command {
	return &cli.Command{
		Name:      "history",
		Usage:     "이슈의 이벤트 이력을 출력한다",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "count", Aliases: []string{"n"}, Usage: "출력 개수"},
		},
		Action: action(runHistory),
	}
}

// editCommand — 메타데이터를 수정한다. 상태는 바꾸지 않는다 (§2).
func editCommand() *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     "이슈 메타데이터를 수정한다",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "title"},
			&cli.StringFlag{Name: "priority", Usage: "high|med|low|none"},
			&cli.StringSliceFlag{Name: "add-label"},
			&cli.StringSliceFlag{Name: "remove-label"},
			&cli.StringSliceFlag{Name: "add-depends-on"},
			&cli.StringSliceFlag{Name: "remove-depends-on"},
			&cli.StringFlag{Name: "body-file"},
			&cli.StringFlag{Name: "plan"},
			&cli.StringFlag{Name: "phase"},
			&cli.IntFlag{Name: "seq"},
			&cli.BoolFlag{Name: "clear-plan", Usage: "plan/phase/seq 해제"},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("edit")
		},
	}
}
