package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	hookEventNew         = "new"
	hookEventAdd         = "add"
	hookEventStart       = "start"
	hookEventDone        = "done"
	hookEventReset       = "reset"
	hookEventConditional = "conditional"
	hookEventPlanDone    = "plan_done"
)

var hookEvents = map[string]bool{
	hookEventNew:         true,
	hookEventAdd:         true,
	hookEventStart:       true,
	hookEventDone:        true,
	hookEventReset:       true,
	hookEventConditional: true,
	hookEventPlanDone:    true,
}

func (value hookConfig) commands(when, event string) []string {
	var rules []hookRule
	switch when {
	case "before":
		rules = value.Before
	case "after":
		rules = value.After
	default:
		return nil
	}
	commands := []string{}
	for _, rule := range rules {
		for _, candidate := range rule.On {
			if strings.TrimSpace(candidate) == event && strings.TrimSpace(rule.Run) != "" {
				commands = append(commands, rule.Run)
				break
			}
		}
	}
	return commands
}

func validateHooks(value hookConfig) error {
	for _, group := range []struct {
		name  string
		rules []hookRule
	}{
		{name: "before", rules: value.Before},
		{name: "after", rules: value.After},
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
				if !hookEvents[event] {
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

func runConfiguredHooks(repoRoot string, settings config, when, event, planDirectory string, phaseID int, status string) error {
	for index, command := range settings.Hooks.commands(when, event) {
		label := fmt.Sprintf("%s %s hook #%d", when, event, index+1)
		if err := runHook(repoRoot, command, label, event, planDirectory, phaseID, status); err != nil {
			return err
		}
	}
	return nil
}

func runHook(repoRoot, command, label, event, planDirectory string, phaseID int, status string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	hook := exec.Command("sh", "-c", command)
	hook.Dir = repoRoot
	hook.Env = append(os.Environ(),
		"PLANR_EVENT="+event,
		"PLANR_PLAN="+planDirectory,
		"PLANR_PHASE="+phaseEnvironmentValue(phaseID),
		"PLANR_STATUS="+status,
	)
	output, err := hook.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%s failed: %w\n%s", label, err, message)
		}
		return fmt.Errorf("%s failed: %w", label, err)
	}
	if text := strings.TrimSpace(string(output)); text != "" {
		fmt.Printf("%s: %s\n", label, text)
	}
	return nil
}

func phaseEnvironmentValue(phaseID int) string {
	if phaseID < 0 {
		return ""
	}
	return strconv.Itoa(phaseID)
}
