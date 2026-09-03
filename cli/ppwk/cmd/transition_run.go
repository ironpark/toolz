package cmd

import (
	"github.com/ironpark/toolz/cli/ppwk/internal/board"
)

// runTransition 은 상태 전이 명령 하나를 실행한다 (§3).
func runTransition(action board.Action) func(*ctx) error {
	return func(x *ctx) error {
		// release --mine 은 인자를 받지 않는다. 유일한 예외다.
		mine := action == board.ActionRelease && x.cmd.Bool("mine")
		if mine {
			if x.cmd.NArg() != 0 {
				return UsageError("--mine 과 이슈 ID 는 함께 쓸 수 없습니다")
			}
		} else if x.cmd.NArg() != 1 {
			return UsageError("이슈 ID 가 필요합니다")
		}

		opts := board.TransitionOptions{
			Message: x.cmd.String("message"),
			Force:   x.cmd.Bool("force"),
			Retry:   x.cmd.Int("retry"),
		}
		if action == board.ActionBlock {
			opts.On = x.cmd.String("on")
		}
		if opts.Retry < 0 {
			return UsageError("--retry 는 0 이상이어야 합니다")
		}

		b, err := x.board()
		if err != nil {
			return err
		}

		if mine {
			released, err := b.ReleaseMine(opts)
			if err != nil {
				return err
			}
			return x.emitIssues(released)
		}

		issue, err := b.Transition(action, x.cmd.Args().First(), opts)
		if err != nil {
			return err
		}
		if x.json {
			return x.emit(issue.Issue)
		}
		x.printf("%s  %s\n", issue.ID, issue.Status)
		return nil
	}
}

// emitIssues 는 여러 이슈의 결과를 낸다 (release --mine).
func (x *ctx) emitIssues(issues []*board.Issue) error {
	if x.json {
		return x.emit(issueDocs(issues))
	}
	for _, issue := range issues {
		x.printf("%s  %s\n", issue.ID, issue.Status)
	}
	return nil
}

// issueDocs 는 --json 이 낼 문서 배열이다. nil 이 아니라 빈 배열이 나와야
// 소비자가 length 만 보면 된다.
func issueDocs(issues []*board.Issue) []any {
	docs := make([]any, 0, len(issues))
	for _, issue := range issues {
		docs = append(docs, issue.Issue)
	}
	return docs
}

// runHistory 는 이슈의 이벤트 이력을 낸다 (§2, §5.3).
func runHistory(x *ctx) error {
	if x.cmd.NArg() != 1 {
		return UsageError("이슈 ID 가 필요합니다")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	events, err := b.History(x.cmd.Args().First(), x.cmd.Int("count"))
	if err != nil {
		return err
	}
	if x.json {
		if events == nil {
			events = []board.Event{}
		}
		return x.emit(events)
	}

	rows := make([][]string, 0, len(events))
	for _, e := range events {
		rows = append(rows, []string{e.Short, e.When, e.Who, e.Subject})
	}
	return x.table(rows)
}
