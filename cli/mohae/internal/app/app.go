// Package app contains mohae's application behavior. The executable package
// only resolves build metadata and hands control to this package.
package app

import (
	"errors"
	"fmt"

	command "github.com/ironpark/toolz/cli/mohae/cmd"
	"github.com/urfave/cli/v3"
)

// NewCommand assembles the complete CLI with build metadata injected into the
// operations that record it in agent sessions and reports.
func NewCommand(version string) *cli.Command {
	return command.New(command.Options{
		Version:               version,
		DefaultReportDir:      DefaultReportDir,
		DefaultTimeoutSeconds: DefaultTimeoutSeconds,
		Actions: command.Actions{
			Run: runAction(version), Compare: compareAction, Web: webAction,
			Init: initAction, Verify: verifyAction, Report: reportAction,
		},
	})
}

// errNotImplemented marks a command whose flags and validation exist but whose
// execution is not written yet.
var errNotImplemented = errors.New("not implemented yet")

func notImplemented(what string) error {
	return fmt.Errorf("%s: %w", what, errNotImplemented)
}
