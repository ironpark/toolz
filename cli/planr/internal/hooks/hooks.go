package hooks

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/agentenv"
)

const DefaultTimeout = 10 * time.Minute

// Config is the hooks block of .planr.yaml.
type Config struct {
	Before  []Rule        `yaml:"before"`
	After   []Rule        `yaml:"after"`
	Timeout time.Duration `yaml:"timeout"`
}

// Rule binds one shell command to the events that trigger it.
type Rule struct {
	On  []string `yaml:"on"`
	Run string   `yaml:"run"`
}

const (
	EventNew         = "new"
	EventAdd         = "add"
	EventPhaseAdd    = "phase_add"
	EventStart       = "start"
	EventDone        = "done"
	EventReset       = "reset"
	EventConditional = "conditional"
	EventPlanDone    = "plan_done"
)

var events = map[string]bool{
	EventNew:         true,
	EventAdd:         true,
	EventPhaseAdd:    true,
	EventStart:       true,
	EventDone:        true,
	EventReset:       true,
	EventConditional: true,
	EventPlanDone:    true,
}

// Commands lists the shell commands bound to when/event.
func (c Config) Commands(when, event string) []string {
	var rules []Rule
	switch when {
	case "before":
		rules = c.Before
	case "after":
		rules = c.After
	default:
		return nil
	}
	Commands := []string{}
	for _, rule := range rules {
		for _, candidate := range rule.On {
			if strings.TrimSpace(candidate) == event && strings.TrimSpace(rule.Run) != "" {
				Commands = append(Commands, rule.Run)
				break
			}
		}
	}
	return Commands
}

func Validate(c Config) error {
	if c.Timeout < 0 {
		return fmt.Errorf("hooks.timeout must not be negative")
	}
	for _, group := range []struct {
		name  string
		rules []Rule
	}{
		{name: "before", rules: c.Before},
		{name: "after", rules: c.After},
	} {
		for index, rule := range group.rules {
			if len(rule.On) == 0 {
				return fmt.Errorf("hooks.%s[%d].on must contain at least one event", group.name, index)
			}
			if strings.TrimSpace(rule.Run) == "" {
				return fmt.Errorf("hooks.%s[%d].run must not be empty", group.name, index)
			}
			seen := map[string]bool{}
			for _, event := range rule.On {
				event = strings.TrimSpace(event)
				if !events[event] {
					return fmt.Errorf("hooks.%s[%d].on contains unknown event %q", group.name, index, event)
				}
				if seen[event] {
					return fmt.Errorf("hooks.%s[%d].on contains duplicate event %q", group.name, index, event)
				}
				seen[event] = true
			}
		}
	}
	return nil
}

func Run(repoRoot string, c Config, skip bool, when, event, planDirectory string, phaseID int, status string, outputWriter io.Writer) error {
	if skip {
		return nil
	}
	for index, command := range c.Commands(when, event) {
		label := fmt.Sprintf("%s %s hook #%d", when, event, index+1)
		if err := runOneTo(repoRoot, command, label, event, planDirectory, phaseID, status, c.TimeoutDuration(), outputWriter); err != nil {
			return err
		}
	}
	return nil
}

// TimeoutDuration is the budget for a single hook. Hooks routinely run test
// suites, so the default is generous, but an unbounded hook would hang planr
// forever with no indication of which command is stuck. A repository can
// override it with hooks.timeout.
func (c Config) TimeoutDuration() time.Duration {
	if c.Timeout <= 0 {
		return DefaultTimeout
	}
	return c.Timeout
}

func runOneTo(repoRoot, command, label, event, planDirectory string, phaseID int, status string, timeout time.Duration, outputWriter io.Writer) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	hook := exec.CommandContext(ctx, "sh", "-c", command)
	hook.Dir = repoRoot
	hook.Env = append(os.Environ(),
		"PLANR_EVENT="+event,
		"PLANR_PLAN="+planDirectory,
		"PLANR_PHASE="+phaseEnvironmentValue(phaseID),
		"PLANR_STATUS="+status,
	)
	// The agent variables describe the process planr itself runs in rather than
	// the event, so they come from the shared detection in agent.go.
	hook.Env = append(hook.Env, agentenv.Environment()...)
	output, err := hook.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("timed out after %s", timeout)
		}
		if message != "" {
			return fmt.Errorf("%s failed: %w\n%s", label, err, message)
		}
		return fmt.Errorf("%s failed: %w", label, err)
	}
	if text := strings.TrimSpace(string(output)); text != "" {
		_, _ = fmt.Fprintf(outputWriter, "%s: %s\n", label, text)
	}
	return nil
}

func phaseEnvironmentValue(phaseID int) string {
	if phaseID < 0 {
		return ""
	}
	return strconv.Itoa(phaseID)
}
