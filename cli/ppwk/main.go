package main

import (
	"context"
	"encoding/json"
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

	code := cmd.ExitGeneral
	var coder cli.ExitCoder
	if errors.As(err, &coder) {
		code = coder.ExitCode()
	}

	// --json 이면 오류도 같은 봉투에 담는다 (features §0.4).
	if root.Bool("json") {
		writeJSONError(stdout, err, code)
		return code
	}
	fmt.Fprintln(stderr, err)
	return code
}

// writeJSONError 는 {"ok":false,"error":{...}} 를 낸다.
func writeJSONError(stdout io.Writer, err error, code int) {
	kind := "error"
	var typed *cmd.Error
	if errors.As(err, &typed) {
		kind = typed.Kind
	}
	payload := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    code,
			"kind":    kind,
			"message": err.Error(),
		},
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
