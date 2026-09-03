package cmd

import (
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/urfave/cli/v3"
)

// planCommand — plan 과 phase 관리 (§5).
func planCommand() *cli.Command {
	return &cli.Command{
		Name:  "plan",
		Usage: "plan 과 phase 를 관리한다",
		Commands: []*cli.Command{
			{
				Name:      "new",
				Usage:     "plan 을 만든다",
				ArgsUsage: "<title>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "priority", Usage: "high|med|low|none"},
					&cli.StringFlag{Name: "id", Usage: "plan ID 직접 지정"},
				},
				Action: action(runPlanNew),
			},
			{
				Name:  "list",
				Usage: "plan 목록을 출력한다",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "status", Usage: "active|closed|cancelled"},
				},
				Action: action(runPlanList),
			},
			{
				Name:      "show",
				Usage:     "진행률과 현재 phase 를 파생 계산해 출력한다",
				ArgsUsage: "<plan>",
				Action:    action(runPlanShow),
			},
			{
				Name:      "advance",
				Usage:     "manual gate 를 개방한다",
				ArgsUsage: "<plan> <phase>",
				Action:    action(runPlanAdvance),
			},
			{
				Name:      "close",
				Usage:     "plan 을 닫는다",
				ArgsUsage: "<plan>",
				Action:    action(planStatusRunner(model.PlanClosed)),
			},
			{
				Name:      "cancel",
				Usage:     "plan 을 취소한다",
				ArgsUsage: "<plan>",
				Action:    action(planStatusRunner(model.PlanCancelled)),
			},
			{
				Name:      "edit",
				Usage:     "plan 메타데이터를 수정한다",
				ArgsUsage: "<plan>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "title"},
					&cli.StringFlag{Name: "priority", Usage: "high|med|low|none"},
				},
				Action: action(runPlanEdit),
			},
			planPhaseCommand(),
		},
	}
}

// planPhaseCommand — phase 하위 명령 (§5).
func planPhaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "phase",
		Usage: "phase 를 관리한다",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "phase 를 추가한다",
				ArgsUsage: "<plan> <title>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "gate", Value: "all_done", Usage: "all_done|any_done|manual"},
					&cli.StringFlag{Name: "id", Usage: "phase ID 직접 지정"},
					&cli.StringFlag{Name: "before", Usage: "이 phase 앞에 삽입"},
					&cli.StringFlag{Name: "after", Usage: "이 phase 뒤에 삽입"},
				},
				Action: action(runPhaseAdd),
			},
			{
				Name:      "edit",
				Usage:     "phase 를 수정한다",
				ArgsUsage: "<plan> <phase>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "title"},
					&cli.StringFlag{Name: "gate", Usage: "all_done|any_done|manual"},
				},
				Action: action(runPhaseEdit),
			},
			{
				Name:      "remove",
				Usage:     "phase 를 제거한다. 소속 task 가 있으면 거부",
				ArgsUsage: "<plan> <phase>",
				Action:    action(runPhaseRemove),
			},
		},
	}
}
