// Package cmd defines mohae's command tree and flags. Runtime behavior is
// injected as action functions so this package stays independent of the
// runner's configuration and trial types.
package cmd

import "github.com/urfave/cli/v3"

type Actions struct {
	Run     cli.ActionFunc
	Compare cli.ActionFunc
	Web     cli.ActionFunc
	Init    cli.ActionFunc
	Verify  cli.ActionFunc
	Report  cli.ActionFunc
}

type Options struct {
	Version               string
	DefaultReportDir      string
	DefaultTimeoutSeconds int
	Actions               Actions
}

func New(options Options) *cli.Command {
	return &cli.Command{
		Name:                  "mohae",
		Usage:                 "automated evaluation and benchmark CLI for AI agents, MCP servers, and CLI skills",
		Version:               options.Version,
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			NewRun(options.Actions.Run, options.DefaultReportDir, options.DefaultTimeoutSeconds),
			NewCompare(options.Actions.Compare, options.DefaultReportDir),
			NewWeb(options.Actions.Web, options.DefaultReportDir),
			NewInit(options.Actions.Init),
			NewVerify(options.Actions.Verify),
			NewReport(options.Actions.Report),
		},
	}
}
