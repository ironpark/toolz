package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ironpark/toolz/cli/ppwk/cmd"
	"github.com/urfave/cli/v3"
)

// 빌드 시 -ldflags 로 주입한다.
var (
	version       = "dev"
	schemaVersion = "1"
)

func main() {
	os.Exit(run(context.Background(), os.Args, os.Stdout, os.Stderr))
}

// run 은 종료 코드를 돌려준다 (§0.3).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := cmd.New(cmd.Version{CLI: version, Schema: schemaVersion}, stdout, stderr)

	err := root.Run(ctx, args)
	if err == nil {
		return cmd.ExitOK
	}
	fmt.Fprintln(stderr, err)

	var coder cli.ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return cmd.ExitGeneral
}
