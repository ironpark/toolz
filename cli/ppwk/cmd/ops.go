package cmd

import (
	"context"

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
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("export")
		},
	}
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
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("fsck")
		},
	}
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
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("archive")
		},
	}
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
