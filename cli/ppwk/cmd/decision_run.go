package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
)

// runDecide 는 결정을 기록한다 (§5.5).
func runDecide(x *ctx) error {
	if x.cmd.NArg() != 1 {
		return UsageError("제목이 필요합니다")
	}
	var body []byte
	if path := x.cmd.String("body-file"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("근거 파일을 읽을 수 없습니다: %w", err)
		}
		body = data
	}

	b, err := x.board()
	if err != nil {
		return err
	}
	opts := board.DecideOptions{
		Title:        x.cmd.Args().First(),
		Context:      x.cmd.String("context"),
		Options:      x.cmd.StringSlice("option"),
		Chosen:       x.cmd.String("decision"),
		Consequences: x.cmd.String("consequences"),
		Issues:       x.cmd.StringSlice("issue"),
		Plan:         x.cmd.String("plan"),
		Supersedes:   x.cmd.String("supersedes"),
		Body:         body,
	}
	decision, err := b.Decide(opts)
	if err != nil {
		return err
	}
	warnDecision(x, opts)

	if x.json {
		return x.emit(decision.Decision)
	}
	x.printf("%s\n", decision.ID)
	return nil
}

// warnDecision 은 기록은 하되 미심쩍은 것을 그 자리에서 알린다.
//
// fsck 로 미루지 않는 이유는 결정이 불변이기 때문이다. 나중에 발견해도 고칠
// 수 없으므로, 아직 다시 쓸 수 있는 지금 말해야 한다.
func warnDecision(x *ctx, opts board.DecideOptions) {
	switch {
	case len(opts.Options) == 0:
		fmt.Fprintln(x.stderr, "warning: 검토한 선택지가 없습니다 (--option)")
	case opts.Chosen != "" && !containsString(opts.Options, opts.Chosen):
		fmt.Fprintf(x.stderr, "warning: 택한 것 %q 가 --option 목록에 없습니다\n", opts.Chosen)
	}
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// runDecisions 는 결정 목록을 낸다. 기본은 유효한 것만이다.
func runDecisions(x *ctx) error {
	// 알 수 없는 하위 명령이 조용히 목록으로 떨어지면 오타가 성공으로 보인다.
	if x.cmd.NArg() > 0 {
		return UsageError("알 수 없는 인자입니다: %v", x.cmd.Args().Slice())
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	entries, err := b.ListDecisions(board.DecisionListOptions{
		All:    x.cmd.Bool("all"),
		Issue:  x.cmd.String("issue"),
		Plan:   x.cmd.String("plan"),
		Search: x.cmd.String("search"),
	})
	if err != nil {
		return err
	}
	if x.json {
		if entries == nil {
			entries = []board.DecisionEntry{}
		}
		return x.emit(entries)
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{entry.ID, entry.Title, decisionNote(entry)})
	}
	return x.table(rows)
}

func decisionNote(entry board.DecisionEntry) string {
	if entry.Superseded() {
		return "superseded by " + strings.Join(entry.SupersededBy, ", ")
	}
	return ""
}

// runDecisionShow 는 결정 하나를 낸다.
func runDecisionShow(x *ctx) error {
	if x.cmd.NArg() != 1 {
		return UsageError("결정 ID 가 필요합니다")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	id := x.cmd.Args().First()
	decision, err := b.ShowDecision(id)
	if err != nil {
		return err
	}
	// superseded_by 는 저장된 값이 아니다. 목록 전체에서 계산한다 (§3.9).
	entries, err := b.ListDecisions(board.DecisionListOptions{All: true})
	if err != nil {
		return err
	}
	var supersededBy []string
	for _, entry := range entries {
		if entry.ID == id {
			supersededBy = entry.SupersededBy
		}
	}

	if x.json {
		return x.emit(map[string]any{"decision": decision.Decision, "superseded_by": supersededBy})
	}
	x.printf("%s\n", decision.Markdown(supersededBy))
	return nil
}

// runDecisionHistory 는 supersedes 체인을 낸다.
func runDecisionHistory(x *ctx) error {
	if x.cmd.NArg() != 1 {
		return UsageError("결정 ID 가 필요합니다")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	chain, err := b.DecisionHistory(x.cmd.Args().First())
	if err != nil {
		return err
	}
	if x.json {
		docs := make([]any, 0, len(chain))
		for _, decision := range chain {
			docs = append(docs, decision.Decision)
		}
		return x.emit(docs)
	}
	rows := make([][]string, 0, len(chain))
	for _, decision := range chain {
		rows = append(rows, []string{decision.ID, decision.DecidedAt.String(), decision.Title})
	}
	return x.table(rows)
}
