// Package cmd defines mohae's command tree and application entry points.
package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

type Options struct {
	Version string
}

func New(options Options) *cli.Command {
	return &cli.Command{
		Name:                  "mohae",
		Usage:                 "automated evaluation and benchmark CLI for AI agents, MCP servers, and CLI skills",
		Version:               options.Version,
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			NewRun(options.Version),
			NewCompare(),
			NewWeb(),
			NewInit(),
			NewVerify(),
			NewReport(),
		},
	}
}

var errNotImplemented = errors.New("not implemented yet")

func notImplemented(what string) error {
	return fmt.Errorf("%s: %w", what, errNotImplemented)
}

func checkFlagValue(flag, value string, allowed []string) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	return fmt.Errorf("unknown --%s %q (one of: %s)", flag, value, strings.Join(allowed, ", "))
}
