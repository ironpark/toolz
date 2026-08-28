// Package cliflag holds the flag builders shared by the command packages, so a
// flag that appears on several commands has one spelling and one description.
package cliflag

import ucli "github.com/urfave/cli/v3"

// JSON is on every command with machine-readable output.
func JSON() ucli.Flag {
	return &ucli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"}
}

// Section selects one editable region of a plan document.
func Section() ucli.Flag {
	return &ucli.StringFlag{Name: "section", Usage: "goals, context, or plan"}
}

// Force overrides a refusal. usage states what is being overridden, since each
// command refuses for a different reason.
func Force(usage string) ucli.Flag {
	return &ucli.BoolFlag{Name: "force", Usage: usage}
}
