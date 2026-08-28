package cli

import (
	"runtime/debug"

	ucli "github.com/urfave/cli/v3"
)

// version is overridable at link time
// (-ldflags "-X github.com/ironpark/toolz/cli/planr/cli.version=v1.2.3") for
// release builds. Installs made with `go install ...@latest` leave it unset and
// fall back to the module version the binary was built from.
var version = ""

func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "unknown"
	}
	return info.Main.Version
}

// jsonFlag is on every command that has machine-readable output. The flag name
// and wording are shared so scripts see one spelling across the whole CLI.
func jsonFlag() ucli.Flag {
	return &ucli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"}
}

// sectionFlag selects one editable region of a plan document.
func sectionFlag() ucli.Flag {
	return &ucli.StringFlag{Name: "section", Usage: "goals, context, or plan"}
}

// forceFlag overrides a refusal. usage states what is being overridden, since
// each command refuses for a different reason.
func forceFlag(usage string) ucli.Flag {
	return &ucli.BoolFlag{Name: "force", Usage: usage}
}
