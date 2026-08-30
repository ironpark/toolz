package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v3"
)

// checkStatus is the outcome of one check. A warning is something a trial can
// run despite, which is exactly why --strict exists.
type checkStatus int

const (
	statusPass checkStatus = iota
	statusWarn
	statusFail
)

func (s checkStatus) icon() string {
	switch s {
	case statusPass:
		return "ok  "
	case statusWarn:
		return "warn"
	default:
		return "fail"
	}
}

type checkResult struct {
	Status checkStatus
	Name   string
	Detail string
}

func verifyAction(_ context.Context, cmd *cli.Command) error {
	configs, err := loadConfigs(cmd.Args().Slice())
	if err != nil {
		// A config that does not parse is itself the verification failure, and
		// is more useful reported as one than as a load error.
		return err
	}
	failed, warned := 0, 0
	for index, config := range configs {
		if index > 0 {
			fmt.Println()
		}
		fmt.Printf("%s (%s)\n", config.Name, config.Path)
		for _, result := range verifyConfig(cmd, config) {
			detail := ""
			if result.Detail != "" {
				detail = "  " + result.Detail
			}
			fmt.Printf("  %s %s%s\n", result.Status.icon(), result.Name, detail)
			switch result.Status {
			case statusFail:
				failed++
			case statusWarn:
				warned++
			}
		}
	}
	fmt.Printf("\n%d config(s), %d failed, %d warning(s)\n", len(configs), failed, warned)
	if failed > 0 {
		return fmt.Errorf("verification failed")
	}
	if warned > 0 && cmd.Bool("strict") {
		return fmt.Errorf("verification produced warnings and --strict is set")
	}
	return nil
}

func verifyConfig(cmd *cli.Command, config *Config) []checkResult {
	results := []checkResult{}
	for _, referenced := range config.ReferencedPaths() {
		info, err := os.Stat(referenced.Path)
		switch {
		case err != nil:
			results = append(results, checkResult{statusFail, referenced.Field, referenced.Path + " does not exist"})
		case referenced.Field == "workspace.source" && !info.IsDir():
			results = append(results, checkResult{statusFail, referenced.Field, referenced.Path + " is not a directory"})
		default:
			results = append(results, checkResult{statusPass, referenced.Field, ""})
		}
	}
	if len(config.Verify.Commands) == 0 {
		// A trial with nothing to grade it still produces a transcript, but no
		// verdict; that is a choice, not necessarily a mistake.
		results = append(results, checkResult{statusWarn, "verify.commands", "not set: the trial will have no pass/fail verdict"})
	}
	if config.Workspace.AgentMD == "" {
		results = append(results, checkResult{statusWarn, "workspace.agent_md", "not set: the agent gets no instructions"})
	}
	if cmd.Bool("check-scripts") {
		results = append(results, checkScripts(config)...)
	}
	if cmd.Bool("check-agent-md") {
		results = append(results, checkAgentMarkdown(config))
	}
	if cmd.Bool("check-mcp") {
		results = append(results, checkResult{statusWarn, "mcp", "--check-mcp is not implemented yet"})
	}
	return results
}

func checkScripts(config *Config) []checkResult {
	results := []checkResult{}
	// Verify commands are shell text rather than files, so only the init
	// script has an executable bit to check.
	for _, script := range []LabeledPath{{"workspace.init_script", config.Workspace.InitScript}} {
		if script.Path == "" {
			continue
		}
		path := config.Resolve(script.Path)
		info, err := os.Stat(path)
		if err != nil {
			continue // Already reported as a missing referenced path.
		}
		if info.Mode()&0o111 == 0 {
			results = append(results, checkResult{statusFail, script.Field, path + " is not executable (chmod +x)"})
			continue
		}
		if _, err := exec.LookPath("bash"); err == nil {
			// `bash -n` parses without running, which is the only way to catch
			// a syntax error before it aborts a trial that already ran.
			if output, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
				results = append(results, checkResult{statusFail, script.Field, strings.TrimSpace(string(output))})
				continue
			}
		}
		results = append(results, checkResult{statusPass, script.Field + " syntax", ""})
	}
	return results
}

func checkAgentMarkdown(config *Config) checkResult {
	if config.Workspace.AgentMD == "" {
		return checkResult{statusWarn, "workspace.agent_md", "not set"}
	}
	path := config.Resolve(config.Workspace.AgentMD)
	data, err := os.ReadFile(path)
	if err != nil {
		return checkResult{statusFail, "workspace.agent_md", err.Error()}
	}
	if strings.TrimSpace(string(data)) == "" {
		return checkResult{statusFail, "workspace.agent_md", path + " is empty"}
	}
	if !strings.Contains(string(data), "#") {
		return checkResult{statusWarn, "workspace.agent_md", "no Markdown headings; the agent may not find its sections"}
	}
	return checkResult{statusPass, "workspace.agent_md content", ""}
}
