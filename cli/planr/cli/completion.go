package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	ucli "github.com/urfave/cli/v3"
)

// planNameShellComplete supplies the bare plan names accepted by commands
// whose first positional argument is a plan. Completion deliberately remains
// silent when configuration or repository discovery is unavailable; shells
// invoke this callback while the user is still typing.
func planNameShellComplete(ctx context.Context, cmd *ucli.Command) {
	args := cmd.Args().Slice()
	if len(args) > 1 {
		// Keep cli's built-in flag completion for the same commands; this
		// callback only adds repository-backed positional values.
		if strings.HasPrefix(args[len(args)-1], "-") {
			ucli.DefaultCompleteWithFlags(ctx, cmd)
		}
		return
	}
	prefix := ""
	if len(args) == 1 {
		prefix = args[0]
		if strings.HasPrefix(prefix, "-") {
			ucli.DefaultCompleteWithFlags(ctx, cmd)
			return
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	planDirectories, err := config.PlanPaths(cwd)
	if err != nil {
		return
	}
	names, err := planNameCompletionValues(planDirectories, prefix)
	if err != nil {
		return
	}
	var writer io.Writer = os.Stdout
	if root := cmd.Root(); root != nil && root.Writer != nil {
		writer = root.Writer
	}
	for _, name := range names {
		_, _ = fmt.Fprintln(writer, name)
	}
}

func planNameCompletionValues(planDirectories []string, prefix string) ([]string, error) {
	seen := map[string]bool{}
	for _, plans := range planDirectories {
		entries, err := os.ReadDir(plans)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := draft.Name(entry.Name())
			if strings.HasPrefix(name, prefix) {
				seen[name] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func isShellCompletionInvocation(args []string) bool {
	if len(args) < 2 || args[len(args)-1] != "--generate-shell-completion" {
		return false
	}
	for _, arg := range args[1 : len(args)-1] {
		if arg == "--" {
			return false
		}
	}
	return true
}
