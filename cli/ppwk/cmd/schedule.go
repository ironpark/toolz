package cmd

import (
	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
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
		Action: action(runNext),
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
			&cli.BoolFlag{Name: "allow-shared-worktree", Usage: "worktree 배타 확보를 건너뜀"},
		},
		Action: action(runReap),
	}
}

// agentsCommand — 잠금 파일을 읽어 에이전트 현황을 출력한다 (§4, D13).
func agentsCommand() *cli.Command {
	return &cli.Command{
		Name:   "agents",
		Usage:  "에이전트 잠금 현황을 출력한다",
		Action: action(runAgents),
	}
}

// runNext 는 다음에 할 이슈를 고른다.
//
// 후보가 없으면 오류가 아니라 빈 결과에 exit 0 이다. 에이전트가 이것을
// "할 일 없음" 으로 읽고 대기하는 것이 정상 흐름이다 (features §4).
func runNext(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	result, err := b.Next(board.NextOptions{
		Plan:        x.cmd.String("plan"),
		Label:       x.cmd.String("label"),
		Claim:       x.cmd.Bool("claim"),
		DryRun:      x.cmd.Bool("dry-run"),
		MaxAttempts: x.cmd.Int("max-attempts"),
	})
	if err != nil {
		return err
	}

	if x.json {
		data := map[string]any{"candidates": issueDocs(result.Candidates)}
		if result.Claimed != nil {
			data["claimed"] = result.Claimed.Issue
		}
		return x.emit(data)
	}
	if result.Claimed != nil {
		x.printf("%s\n", result.Claimed.ID)
		return nil
	}
	rows := make([][]string, 0, len(result.Candidates))
	for _, c := range result.Candidates {
		rows = append(rows, []string{c.ID, string(c.Priority), dash(c.Plan), dash(c.Phase), c.Title})
	}
	return x.table(rows)
}

func runReap(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	issues, err := b.Reap(board.ReapOptions{DryRun: x.cmd.Bool("dry-run")})
	if err != nil {
		return err
	}
	return x.emitIssues(issues)
}

func runAgents(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	agents := b.Agents()
	if x.json {
		if agents == nil {
			agents = []model.Lease{}
		}
		return x.emit(agents)
	}
	rows := make([][]string, 0, len(agents))
	for _, a := range agents {
		rows = append(rows, []string{a.Agent, a.Session, a.Worktree, a.LastActivity.String()})
	}
	return x.table(rows)
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
				Action: action(runSessionEvent),
			},
		},
	}
}
