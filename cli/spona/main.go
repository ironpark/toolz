package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"
)

const version = "dev"

const helpTemplate = `사용법: spona [--version]

에이전트 실행 환경을 프리셋으로 저장하고 실행합니다.

옵션:
{{range .VisibleFlags}}   {{.}}
{{end}}`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newCommand(stderr io.Writer) *cli.Command {
	return &cli.Command{
		Name:                          "spona",
		Usage:                         "에이전트 실행 환경을 프리셋으로 저장하고 실행합니다.",
		Version:                       version,
		Writer:                        stderr,
		ErrWriter:                     stderr,
		CustomRootCommandHelpTemplate: helpTemplate,
		HideHelpCommand:               true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 0 {
				return fmt.Errorf("알 수 없는 인자: %s", cmd.Args().First())
			}
			if err := cli.ShowRootCommandHelp(cmd); err != nil {
				return err
			}
			return errors.New("명령을 지정해 주세요")
		},
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cli.HelpFlag = &cli.BoolFlag{Name: "help", Aliases: []string{"h"}, Usage: "도움말 출력"}
	cli.VersionFlag = &cli.BoolFlag{Name: "version", Aliases: []string{"v"}, Usage: "버전 출력"}
	cli.VersionPrinter = func(cmd *cli.Command) {
		fmt.Fprintf(stdout, "spona %s\n", cmd.Root().Version)
	}
	return newCommand(stderr).Run(context.Background(), append([]string{"spona"}, args...))
}
