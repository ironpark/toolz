package cmd

import (
	"fmt"
	"strings"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// planArg 는 <plan> 인자 하나를 읽는다.
func planArg(x *ctx, want int, usage string) ([]string, error) {
	if x.cmd.NArg() != want {
		return nil, UsageError("%s", usage)
	}
	return x.cmd.Args().Slice(), nil
}

// emitPlan 은 plan 하나를 낸다.
func (x *ctx) emitPlan(plan *board.Plan) error {
	if x.json {
		return x.emit(plan.Plan)
	}
	x.printf("%s\n", plan.ID)
	return nil
}

func runPlanNew(x *ctx) error {
	args, err := planArg(x, 1, "제목이 필요합니다")
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	plan, err := b.AddPlan(board.PlanAddOptions{
		Title:    args[0],
		Priority: model.Priority(x.cmd.String("priority")),
		ID:       x.cmd.String("id"),
	})
	if err != nil {
		return err
	}
	return x.emitPlan(plan)
}

func runPlanList(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	plans, err := b.ListPlans(model.PlanStatus(x.cmd.String("status")))
	if err != nil {
		return err
	}
	if x.json {
		docs := make([]any, 0, len(plans))
		for _, plan := range plans {
			docs = append(docs, plan.Plan)
		}
		return x.emit(docs)
	}
	rows := make([][]string, 0, len(plans))
	for _, plan := range plans {
		rows = append(rows, []string{plan.ID, string(plan.Status), string(plan.Priority),
			fmt.Sprintf("%d phases", len(plan.Phases)), plan.Title})
	}
	return x.table(rows)
}

// runPlanShow 는 진행률과 현재 phase 를 파생 계산해 낸다 (features §5).
func runPlanShow(x *ctx) error {
	args, err := planArg(x, 1, "plan ID 가 필요합니다")
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	view, err := b.ShowPlanView(args[0])
	if err != nil {
		return err
	}
	if x.json {
		return x.emit(view)
	}

	x.printf("%s  %s  [%s]  %d/%d\n", view.Plan.ID, view.Plan.Title, view.Plan.Status,
		view.Done, view.Total)
	for _, phase := range view.Phases {
		state := phase.State
		// gate 로 막힌 것은 표시상의 파생값이다. 소속 task 의 status 는
		// 그대로 open 이다 (§3.7.5).
		if !phase.Open {
			state = fmt.Sprintf("blocked (gate: %s)", phase.Gate)
		}
		marker := ""
		if phase.Current {
			marker = "   ← 현재 phase"
		}
		x.printf("\n  %s  %s  %d/%d  %s%s\n", phase.ID, phase.Title,
			phase.Done, phase.Total, state, marker)
		rows := make([][]string, 0, len(phase.Tasks))
		for _, task := range phase.Tasks {
			rows = append(rows, []string{"     ", task.ID, string(task.Status), dash(task.Owner), task.Title})
		}
		if err := x.table(rows); err != nil {
			return err
		}
	}
	return nil
}

func runPlanAdvance(x *ctx) error {
	args, err := planArg(x, 2, "plan 과 phase ID 가 필요합니다")
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	plan, err := b.AdvancePhase(args[0], args[1])
	if err != nil {
		return err
	}
	return x.emitPlan(plan)
}

// planStatusRunner 는 close/cancel 처럼 상태만 바꾸는 명령을 만든다.
func planStatusRunner(status model.PlanStatus) func(*ctx) error {
	return func(x *ctx) error {
		args, err := planArg(x, 1, "plan ID 가 필요합니다")
		if err != nil {
			return err
		}
		b, err := x.board()
		if err != nil {
			return err
		}
		plan, err := b.SetPlanStatus(args[0], status)
		if err != nil {
			return err
		}
		return x.emitPlan(plan)
	}
}

func runPlanEdit(x *ctx) error {
	args, err := planArg(x, 1, "plan ID 가 필요합니다")
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	plan, err := b.EditPlan(args[0], board.PlanEditOptions{
		Title:    x.cmd.String("title"),
		Priority: model.Priority(x.cmd.String("priority")),
	})
	if err != nil {
		return err
	}
	return x.emitPlan(plan)
}

func runPhaseAdd(x *ctx) error {
	if x.cmd.NArg() != 2 {
		return UsageError("plan ID 와 제목이 필요합니다")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	plan, err := b.AddPhase(x.cmd.Args().Get(0), board.PhaseAddOptions{
		Title:  x.cmd.Args().Get(1),
		Gate:   model.Gate(x.cmd.String("gate")),
		ID:     x.cmd.String("id"),
		Before: x.cmd.String("before"),
		After:  x.cmd.String("after"),
	})
	if err != nil {
		return err
	}
	if x.json {
		return x.emit(plan.Plan)
	}
	ids := make([]string, 0, len(plan.Phases))
	for _, phase := range plan.Phases {
		ids = append(ids, phase.ID)
	}
	x.printf("%s\n", strings.Join(ids, " "))
	return nil
}

func runPhaseEdit(x *ctx) error {
	args, err := planArg(x, 2, "plan 과 phase ID 가 필요합니다")
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	plan, err := b.EditPhase(args[0], args[1], board.PhaseEditOptions{
		Title: x.cmd.String("title"),
		Gate:  model.Gate(x.cmd.String("gate")),
	})
	if err != nil {
		return err
	}
	return x.emitPlan(plan)
}

func runPhaseRemove(x *ctx) error {
	args, err := planArg(x, 2, "plan 과 phase ID 가 필요합니다")
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	plan, err := b.RemovePhase(args[0], args[1])
	if err != nil {
		return err
	}
	return x.emitPlan(plan)
}
