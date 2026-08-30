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

func jsonFlag() ucli.Flag {
	return &ucli.BoolFlag{Name: "json", Usage: "write machine-readable JSON"}
}

func sectionFlag() ucli.Flag {
	return &ucli.StringFlag{Name: "section", Usage: "goals, context, or plan"}
}

func forceFlag(usage string) ucli.Flag {
	return &ucli.BoolFlag{Name: "force", Usage: usage}
}
