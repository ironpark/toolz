package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/ironpark/toolz/cli/mohae/internal/app"
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
	if err := app.NewCommand(buildVersion()).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "mohae: %v\n", err)
		os.Exit(1)
	}
}
