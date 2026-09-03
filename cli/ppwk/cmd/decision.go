package cmd

import (
	"github.com/urfave/cli/v3"
)

// decideCommand — 불변 ADR 을 기록한다 (§5.5).
// 수정 명령은 없다. 바꾸려면 --supersedes 로 새 결정을 만든다.
func decideCommand() *cli.Command {
	return &cli.Command{
		Name:      "decide",
		Usage:     "결정을 기록한다",
		ArgsUsage: "<title>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "context", Usage: "배경"},
			&cli.StringSliceFlag{Name: "option", Usage: "검토한 선택지. 반복 가능"},
			&cli.StringFlag{Name: "decision", Usage: "택한 것"},
			&cli.StringFlag{Name: "consequences", Usage: "결과·재검토 조건"},
			&cli.StringSliceFlag{Name: "issue", Usage: "관련 이슈. 반복 가능"},
			&cli.StringFlag{Name: "plan", Usage: "관련 plan"},
			&cli.StringFlag{Name: "supersedes", Usage: "대체하는 이전 결정 ID"},
			&cli.StringFlag{Name: "body-file", Usage: "긴 근거 파일"},
		},
		Action: action(runDecide),
	}
}

// decisionsCommand — 결정을 조회한다 (§5.5).
func decisionsCommand() *cli.Command {
	return &cli.Command{
		Name:  "decisions",
		Usage: "결정을 조회한다. 기본은 유효한 것만",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "all", Usage: "superseded 포함"},
			&cli.StringFlag{Name: "issue", Usage: "이슈와 연결된 결정"},
			&cli.StringFlag{Name: "plan", Usage: "plan 과 연결된 결정"},
			&cli.StringFlag{Name: "search", Usage: "제목·본문 검색"},
		},
		Action: action(runDecisions),
		Commands: []*cli.Command{
			{
				Name:      "show",
				Usage:     "결정 하나를 출력한다",
				ArgsUsage: "<id>",
				Action:    action(runDecisionShow),
			},
			{
				Name:      "history",
				Usage:     "supersedes 체인을 출력한다",
				ArgsUsage: "<id>",
				Action:    action(runDecisionHistory),
			},
		},
	}
}
