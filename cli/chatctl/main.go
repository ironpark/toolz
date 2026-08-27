package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	appcli "github.com/ironpark/toolz/cli/chatctl/internal/cli"
)

// version은 빌드 시 -ldflags 로 주입합니다.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := appcli.New(version).Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}
