package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
	"github.com/urfave/cli/v3"
)

// ctx 는 한 명령 실행에 필요한 것을 모은다.
type ctx struct {
	cmd    *cli.Command
	stdout io.Writer
	stderr io.Writer
	json   bool
	quiet  bool
}

// newCtx 는 전역 플래그를 읽는다.
func newCtx(c *cli.Command) *ctx {
	return &ctx{
		cmd:    c,
		stdout: c.Root().Writer,
		stderr: c.Root().ErrWriter,
		json:   c.Bool("json"),
		quiet:  c.Bool("quiet"),
	}
}

// board 는 보드를 연다.
func (x *ctx) board() (*board.Board, error) {
	path := x.cmd.String("C")
	// 신원의 기본값이 worktree basename 이므로 상대 경로를 그대로 쓰면
	// "agent:." 같은 이름이 나온다.
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return board.OpenFor(path, board.OpenOptions{
		Session: session.Options{
			Flag:     x.cmd.String("agent"),
			Worktree: abs,
		},
		AllowSharedWorktree: x.cmd.Bool("allow-shared-worktree"),
	})
}

// printf 는 --quiet 이면 아무것도 하지 않는다.
func (x *ctx) printf(format string, args ...any) {
	if x.quiet {
		return
	}
	fmt.Fprintf(x.stdout, format, args...)
}

// table 은 정렬된 행들을 낸다. --quiet 이면 아무것도 하지 않는다.
func (x *ctx) table(rows [][]string) error {
	if x.quiet {
		return nil
	}
	w := tabwriter.NewWriter(x.stdout, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

// emit 은 --json 응답을 낸다 (features §0.4).
func (x *ctx) emit(data any) error {
	enc := json.NewEncoder(x.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"ok": true, "data": data})
}
