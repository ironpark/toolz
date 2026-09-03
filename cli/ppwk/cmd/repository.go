package cmd

import (
	"context"

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
		Action: func(_ context.Context, _ *cli.Command) error {
			return notImplemented("init")
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
			_ = v
			return notImplemented("version")
		},
	}
}
