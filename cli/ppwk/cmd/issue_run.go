package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// runAdd 는 이슈를 만든다.
func runAdd(x *ctx) error {
	if x.cmd.NArg() == 0 {
		return UsageError("제목이 필요합니다")
	}
	if x.cmd.NArg() > 1 {
		return UsageError("제목은 하나만 받습니다: %q", x.cmd.Args().Slice())
	}

	body, err := readBody(x)
	if err != nil {
		return err
	}
	if (x.cmd.String("plan") == "") != (x.cmd.String("phase") == "") {
		return UsageError("--plan 과 --phase 는 함께 지정해야 합니다")
	}

	b, err := x.board()
	if err != nil {
		return err
	}
	issue, err := b.Add(board.AddOptions{
		Title:     x.cmd.Args().First(),
		Body:      body,
		Priority:  model.Priority(x.cmd.String("priority")),
		Labels:    x.cmd.StringSlice("label"),
		DependsOn: x.cmd.StringSlice("depends-on"),
		Plan:      x.cmd.String("plan"),
		Phase:     x.cmd.String("phase"),
		Seq:       x.cmd.Int("seq"),
		SeqSet:    x.cmd.IsSet("seq"),
	})
	if err != nil {
		return err
	}
	if x.json {
		return x.emit(issue.Issue)
	}
	x.printf("%s\n", issue.ID)
	return nil
}

// readBody 는 본문을 세 경로 중 하나에서 읽는다.
func readBody(x *ctx) ([]byte, error) {
	text, file, stdin := x.cmd.String("body"), x.cmd.String("body-file"), x.cmd.Bool("body-stdin")

	given := 0
	for _, set := range []bool{text != "", file != "", stdin} {
		if set {
			given++
		}
	}
	if given > 1 {
		return nil, UsageError("--body, --body-file, --body-stdin 중 하나만 쓸 수 있습니다")
	}

	switch {
	case text != "":
		return []byte(ensureTrailingNewline(text)), nil
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("본문 파일을 읽을 수 없습니다: %w", err)
		}
		return data, nil
	case stdin:
		data, err := readAll(x.cmd.Reader)
		if err != nil {
			return nil, fmt.Errorf("stdin 을 읽을 수 없습니다: %w", err)
		}
		return data, nil
	}
	return nil, nil
}

// runList 는 이슈 목록을 낸다.
func runList(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	opts := board.ListOptions{
		Owner:      x.cmd.String("owner"),
		Label:      x.cmd.String("label"),
		Plan:       x.cmd.String("plan"),
		Phase:      x.cmd.String("phase"),
		Unassigned: x.cmd.Bool("unassigned"),
		Archived:   x.cmd.Bool("archived"),
		All:        x.cmd.Bool("all"),
		Limit:      x.cmd.Int("limit"),
	}
	for _, s := range x.cmd.StringSlice("status") {
		opts.Status = append(opts.Status, model.Status(s))
	}
	for _, p := range x.cmd.StringSlice("priority") {
		opts.Priority = append(opts.Priority, model.Priority(p))
	}

	entries, err := b.List(opts)
	if err != nil {
		return err
	}
	if x.json {
		if entries == nil {
			entries = []board.ListEntry{}
		}
		return x.emit(entries)
	}
	if len(entries) == 0 {
		return nil
	}

	w := tabwriter.NewWriter(x.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tOWNER\tPLAN\tPHASE\tTITLE")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Status, dash(e.Owner), dash(e.Plan), dash(e.Phase), e.Title)
	}
	return w.Flush()
}

// runShow 는 이슈 하나를 낸다.
func runShow(x *ctx) error {
	if x.cmd.NArg() != 1 {
		return UsageError("이슈 ID 가 필요합니다")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	issue, err := b.Show(x.cmd.Args().First())
	if err != nil {
		if isNotFound(err) {
			return NotFoundError("%v", err)
		}
		return err
	}
	if x.json {
		return x.emit(issue.Issue)
	}

	x.printf("%s  %s\n\n", issue.ID, issue.Title)
	w := tabwriter.NewWriter(x.stdout, 0, 0, 4, ' ', 0)
	fmt.Fprintf(w, "Status\t%s\n", issue.Status)
	fmt.Fprintf(w, "Owner\t%s\n", dash(issue.Owner))
	fmt.Fprintf(w, "Priority\t%s\n", issue.Priority)
	if issue.Plan != "" {
		fmt.Fprintf(w, "Plan\t%s / %s (seq %d)\n", issue.Plan, issue.Phase, issue.Seq)
	}
	if len(issue.Labels) > 0 {
		fmt.Fprintf(w, "Labels\t%s\n", strings.Join(issue.Labels, ", "))
	}
	if len(issue.DependsOn) > 0 {
		fmt.Fprintf(w, "Depends on\t%s\n", strings.Join(issue.DependsOn, ", "))
	}
	fmt.Fprintf(w, "Created\t%s  by %s\n", issue.CreatedAt, issue.UpdatedBy)
	fmt.Fprintf(w, "Updated\t%s  by %s\n", issue.UpdatedAt, issue.UpdatedBy)
	if err := w.Flush(); err != nil {
		return err
	}
	if len(issue.Body) > 0 {
		x.printf("\n%s", issue.Body)
	}
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// readAll 은 reader 를 끝까지 읽는다. nil 이면 stdin 을 쓴다.
func readAll(r io.Reader) ([]byte, error) {
	if r == nil {
		r = os.Stdin
	}
	return io.ReadAll(r)
}

// isNotFound 는 도메인 계층의 "없음" 을 알아본다.
func isNotFound(err error) bool {
	return errors.Is(err, board.ErrNotFound)
}
