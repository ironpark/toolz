package cmd

import (
	"context"
	"os"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/urfave/cli/v3"
)

// exportCommand — 단방향 파생물을 만든다. 편집해도 반영되지 않는다 (§7).
func exportCommand() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "보드를 내보낸다",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Value: "json", Usage: "json|md|csv"},
			&cli.BoolFlag{Name: "all", Usage: "archive 포함"},
			&cli.BoolFlag{Name: "decisions", Usage: "ADR 마크다운"},
			&cli.StringFlag{Name: "plan", Usage: "특정 plan 만"},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "출력 경로. 기본 stdout"},
		},
		Action: action(runExport),
	}
}

// runExport 는 보드를 단방향 파생물로 내보낸다 (§7).
func runExport(x *ctx) error {
	if x.cmd.Bool("decisions") {
		return notImplemented("export --decisions")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	data, err := b.Export(board.ExportOptions{
		All:  x.cmd.Bool("all"),
		Plan: x.cmd.String("plan"),
	})
	if err != nil {
		return err
	}
	rendered, err := data.Render(x.cmd.String("format"))
	if err != nil {
		return UsageError("%v", err)
	}

	if path := x.cmd.String("output"); path != "" {
		// 0644 다. 파생물이고 비밀이 아니다 — 다만 생성 파일은
		// .gitignore 에 넣으라고 안내한다 (§5.4).
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			return err
		}
		x.printf("%s\n", path)
		return nil
	}
	_, err = x.stdout.Write(rendered)
	return err
}

// importCommand — export --format json 출력을 다시 넣는다 (§7).
func importCommand() *cli.Command {
	return &cli.Command{
		Name:      "import",
		Usage:     "내보낸 JSON 을 되돌려 넣는다",
		ArgsUsage: "<file>",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Usage: "적용하지 않고 확인만"},
			&cli.StringFlag{Name: "format", Value: "json", Usage: "입력 형식"},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("import")
		},
	}
}

// fsckCommand — 무결성을 검사한다 (§7).
// --fix 는 trailer 재생성과 archive 이동만 자동 처리한다.
func fsckCommand() *cli.Command {
	return &cli.Command{
		Name:  "fsck",
		Usage: "보드 무결성을 검사한다",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "fix", Usage: "trailer 재생성과 archive 이동만 자동 수정"},
		},
		Action: action(runFsck),
	}
}

// runFsck 는 무결성을 검사한다 (§9.3).
//
// error 가 하나라도 있으면 exit 1 이다. warn 은 종료 코드를 바꾸지 않는다 —
// 경고로 CI 를 멈추면 아무도 경고를 늘리지 않게 된다.
func runFsck(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	findings, err := b.Fsck(board.FsckOptions{Fix: x.cmd.Bool("fix")})
	if err != nil {
		return err
	}

	if x.json {
		if err := x.emit(map[string]any{"findings": findings}); err != nil {
			return err
		}
	} else {
		rows := make([][]string, 0, len(findings))
		for _, f := range findings {
			rows = append(rows, []string{f.Level, f.Check, dash(f.ID), f.Message, fixNote(f)})
		}
		if err := x.table(rows); err != nil {
			return err
		}
	}
	if board.HasErrors(findings) {
		return &Error{Code: ExitGeneral, Kind: "fsck_failed", Msg: "무결성 검사에서 오류를 찾았습니다"}
	}
	return nil
}

// fixNote 는 --fix 의 결과를 한 마디로 적는다.
func fixNote(f board.Finding) string {
	switch {
	case f.Fixed:
		return "fixed"
	case f.FixError != "":
		return "fix failed: " + f.FixError
	}
	return ""
}

// gcCommand — ref 를 packing 하고 크기를 보고한다 (§7).
func gcCommand() *cli.Command {
	return &cli.Command{
		Name:  "gc",
		Usage: "ref 를 정리하고 크기를 보고한다",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "pack-refs", Usage: "git pack-refs --all 실행"},
			&cli.BoolFlag{Name: "dry-run", Usage: "변경 없이 보고만"},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("gc")
		},
	}
}

// archiveCommand — 종료 상태 이슈를 archive 로 옮긴다 (§7).
// 평소에는 done/cancel 이 자동으로 옮기므로 --sweep 은 복구용이다.
func archiveCommand() *cli.Command {
	return &cli.Command{
		Name:      "archive",
		Usage:     "이슈를 archive 로 옮긴다",
		ArgsUsage: "[id]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "sweep", Usage: "종료 상태인데 issues/ 에 남은 것 일괄 이동"},
		},
		Action: action(runArchive),
	}
}

// runArchive 는 이슈를 archive 로 옮긴다.
func runArchive(x *ctx) error {
	sweep := x.cmd.Bool("sweep")
	if sweep == (x.cmd.NArg() > 0) {
		return UsageError("이슈 ID 하나 또는 --sweep 중 하나가 필요합니다")
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	if sweep {
		moved, err := b.ArchiveSweep()
		if err != nil {
			return err
		}
		return x.emitIssues(moved)
	}
	issue, err := b.Archive(x.cmd.Args().First())
	if err != nil {
		return err
	}
	return x.emitIssues([]*board.Issue{issue})
}

// unarchiveCommand — v1 미지원. 명시적 오류로 거부한다 (§9).
func unarchiveCommand() *cli.Command {
	return &cli.Command{
		Name:      "unarchive",
		Usage:     "v1 미지원",
		ArgsUsage: "<id>",
		Hidden:    true,
		Action: func(_ context.Context, _ *cli.Command) error {
			return UsageError("unarchive 는 v1 에서 지원하지 않습니다. 이력 정합성 판단이 필요합니다")
		},
	}
}
