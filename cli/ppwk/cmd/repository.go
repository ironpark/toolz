package cmd

import (
	"context"
	"fmt"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/urfave/cli/v3"
)

// initCommand — 보드를 초기화한다. 저장소당 한 번 (§1).
func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "보드를 초기화한다",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "hooks", Usage: "reference-transaction hook 설치"},
			&cli.BoolFlag{Name: "force", Usage: "기존 hook 덮어쓰기"},
			&cli.BoolFlag{Name: "no-agents-md", Usage: "에이전트 문서 생성 건너뛰기"},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			return runInit(newCtx(c))
		},
	}
}

// doctorCommand — 환경을 점검한다 (§1).
func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "환경을 점검한다. FAIL 이 있으면 exit 1",
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("doctor")
		},
	}
}

// versionCommand — CLI/스키마/git 버전을 출력한다 (§1).
func versionCommand(v Version) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "CLI·스키마·git 버전을 출력한다",
		Action: func(_ context.Context, c *cli.Command) error {
			x := newCtx(c)
			if x.json {
				return x.emit(map[string]string{"cli": v.CLI, "schema": v.Schema})
			}
			x.printf("ppwk    %s\nschema  %s\n", v.CLI, v.Schema)
			return nil
		},
	}
}

// runInit 은 보드를 초기화한다.
func runInit(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	result, err := b.Init(board.InitOptions{
		Hooks:      x.cmd.Bool("hooks"),
		NoAgentsMD: x.cmd.Bool("no-agents-md"),
		Force:      x.cmd.Bool("force"),
	})
	if err != nil {
		return err
	}
	if x.json {
		return x.emit(result)
	}

	if result.SchemaCreated {
		x.printf("보드를 초기화했습니다.\n")
	} else {
		x.printf("이미 초기화된 보드입니다.\n")
	}
	for _, doc := range result.DocsCreated {
		x.printf("  생성  %s\n", doc)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(x.stderr, "경고: %s\n", w)
	}
	for _, n := range result.Notes {
		x.printf("\n%s\n", n)
	}
	return nil
}
