package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/urfave/cli/v3"
)

// version is overridable at link time (-ldflags "-X main.version=v1.2.3") for
// release builds. Installs made with `go install ...@latest` leave it unset and
// fall back to the module version the binary was built from.
var version = ""

// buildVersion is resolved once: the build info is embedded and immutable, and
// reports ask for the version once per trial and per MCP probe.
var buildVersion = sync.OnceValue(func() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "unknown"
	}
	return info.Main.Version
})

func main() {
	if err := newRootCommand().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "mohae: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cli.Command {
	return &cli.Command{
		Name:                  "mohae",
		Usage:                 "automated evaluation and benchmark CLI for AI agents, MCP servers, and CLI skills",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			newRunCommand(),
			newCompareCommand(),
			newWebCommand(),
			newInitCommand(),
			newVerifyCommand(),
			newReportCommand(),
		},
	}
}

// errNotImplemented marks a command whose flags and configuration handling are
// in place but whose execution is not written yet. It is a distinct error so
// the skeleton fails loudly instead of exiting 0 on work it never did.
var errNotImplemented = errors.New("not implemented yet")

func notImplemented(what string) error {
	return fmt.Errorf("%s: %w", what, errNotImplemented)
}
