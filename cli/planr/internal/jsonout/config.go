package jsonout

import (
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doctor"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
)

type ConfigOutput struct {
	ConfigFile     *string     `json:"config_file"`
	RepositoryRoot string      `json:"repository_root"`
	Agent          string      `json:"agent"`
	Language       string      `json:"language"`
	PlansDirs      []string    `json:"plans_dirs"`
	Ignore         []string    `json:"ignore"`
	Hooks          configHooks `json:"hooks"`
}

type configHooks struct {
	Timeout string     `json:"timeout"`
	Before  []hookRule `json:"before"`
	After   []hookRule `json:"after"`
}

type hookRule struct {
	On  []string `json:"on"`
	Run string   `json:"run"`
}

type InitOutput struct {
	ConfigFile     string   `json:"config_file"`
	RepositoryRoot string   `json:"repository_root"`
	Language       string   `json:"language"`
	PlansDirs      []string `json:"plans_dirs"`
	// Created and Existed are separate so a caller can tell a fresh setup from
	// a rerun without diffing the filesystem itself.
	Created []string `json:"created"`
	Existed []string `json:"existed"`
}

type DoctorOutput struct {
	Issues []doctorIssue `json:"issues"`
}

type doctorIssue struct {
	Location string `json:"location"`
	Message  string `json:"message"`
}

func Init(target, root, language string, plansDirs, created, existed []string) InitOutput {
	return InitOutput{
		ConfigFile:     target,
		RepositoryRoot: root,
		Language:       language,
		PlansDirs:      append([]string{}, plansDirs...),
		// Non-nil slices: `created` and `existed` encode as [] rather than
		// null, so a consumer can iterate them without a nil check.
		Created: append([]string{}, created...),
		Existed: append([]string{}, existed...),
	}
}

func Config(settings config.Config, root, agent string) ConfigOutput {
	var configFile *string
	if settings.Path != "" {
		value := settings.Path
		configFile = &value
	}
	return ConfigOutput{
		ConfigFile:     configFile,
		RepositoryRoot: root,
		Agent:          agent,
		Language:       settings.Language,
		PlansDirs:      append([]string{}, settings.PlansDirs...),
		Ignore:         append([]string{}, settings.Ignore...),
		Hooks: configHooks{
			Timeout: settings.Hooks.TimeoutDuration().String(),
			Before:  hookRules(settings.Hooks.Before),
			After:   hookRules(settings.Hooks.After),
		},
	}
}

func hookRules(rules []hooks.Rule) []hookRule {
	result := make([]hookRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, hookRule{On: append([]string{}, rule.On...), Run: rule.Run})
	}
	return result
}

func Doctor(issues []doctor.Issue) DoctorOutput {
	result := make([]doctorIssue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, doctorIssue{Location: issue.Location, Message: issue.Message})
	}
	return DoctorOutput{Issues: result}
}
