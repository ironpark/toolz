package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/urfave/cli/v3"
)

const defaultHookTimeout = 10 * time.Minute

type config struct {
	PlansDir  string     `yaml:"plans_dir"`
	PlansDirs []string   `yaml:"plans_dirs"`
	Ignore    []string   `yaml:"ignore"`
	Language  string     `yaml:"language"`
	Hooks     hookConfig `yaml:"hooks"`

	// skipHooks is set only for the current CLI invocation by --no-hooks. It
	// never comes from .planr.yaml.
	skipHooks bool `yaml:"-"`

	// configPath is populated only when a configuration file was found. It is
	// intentionally not part of the YAML model; the `config` command uses it to
	// explain which file supplied the effective values.
	configPath string `yaml:"-"`
}

func commandConfig(settings config, cmd *cli.Command) config {
	settings.skipHooks = cmd.Bool("no-hooks")
	return settings
}

type hookConfig struct {
	Before  []hookRule    `yaml:"before"`
	After   []hookRule    `yaml:"after"`
	Timeout time.Duration `yaml:"timeout"`
}

type hookRule struct {
	On  []string `yaml:"on"`
	Run string   `yaml:"run"`
}

func loadConfig(start string) (config, string, error) {
	location, err := discoverConfig(start)
	if err != nil {
		return config{}, "", err
	}
	if location.path == "" {
		return defaultConfig(), location.baseRoot, nil
	}
	value, err := parseConfigFile(location.path)
	if err != nil {
		return config{}, "", err
	}
	value.configPath = location.path
	return value, location.baseRoot, nil
}

// configLocation describes both the root against which relative settings are
// resolved and the file selected by the upward search. When the starting path
// is inside a worktree, the search stops at the worktree root: parent
// directories above it must not unexpectedly contribute another .planr.yaml.
type configLocation struct {
	baseRoot string
	path     string
}

func discoverConfig(start string) (configLocation, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return configLocation{}, err
	}
	absolute = filepath.Clean(absolute)

	searchRoot := absolute
	baseRoot := absolute
	insideWorktree := false
	if repository, openErr := git.PlainOpenWithOptions(absolute, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true}); openErr == nil {
		if worktree, worktreeErr := repository.Worktree(); worktreeErr == nil {
			if root, rootErr := filepath.Abs(worktree.Filesystem.Root()); rootErr == nil {
				root = rootInStartPath(root, absolute)
				searchRoot = filepath.Clean(root)
				baseRoot = searchRoot
				insideWorktree = true
			}
		}
	}

	current := absolute
	for {
		path := filepath.Join(current, ".planr.yaml")
		_, statErr := os.Stat(path)
		if statErr == nil {
			if !insideWorktree {
				baseRoot = current
			}
			return configLocation{baseRoot: baseRoot, path: path}, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			// Preserve the path so parseConfigFile can return the useful read
			// error, instead of silently treating an inaccessible config as absent.
			return configLocation{baseRoot: baseRoot, path: path}, nil
		}
		if current == searchRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return configLocation{baseRoot: baseRoot}, nil
}

// rootInStartPath keeps the spelling of the path supplied by the caller. On
// macOS, for example, /var is commonly an alias for /private/var; go-git may
// return the latter while callers and diagnostics use the former. Comparing
// the resolved paths lets us safely translate the repository root back into
// the caller's path namespace.
func rootInStartPath(root, start string) string {
	root = filepath.Clean(root)
	start = filepath.Clean(start)
	evaluatedRoot, rootErr := filepath.EvalSymlinks(root)
	if rootErr != nil {
		return root
	}
	// EvalSymlinks requires the path to exist. Walk to the nearest existing
	// ancestor so a caller can still get the correct boundary for a path that
	// is about to be created.
	candidate := start
	for {
		evaluatedCandidate, candidateErr := filepath.EvalSymlinks(candidate)
		if candidateErr == nil {
			relative, relErr := filepath.Rel(evaluatedRoot, evaluatedCandidate)
			if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
				translated := candidate
				for relative != "." && relative != "" {
					translated = filepath.Dir(translated)
					relative = filepath.Dir(relative)
				}
				return filepath.Clean(translated)
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		candidate = parent
	}
	return root
}

func defaultConfig() config {
	return config{PlansDirs: []string{"plan"}, Language: doc.DefaultLanguage, Hooks: hookConfig{Timeout: defaultHookTimeout}}
}

func parseConfigFile(path string) (config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var value config
	if err := yaml.UnmarshalWithOptions(contents, &value, yaml.Strict()); err != nil {
		return config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	value, err = normalizeConfig(value)
	if err != nil {
		return config{}, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

func normalizeConfig(value config) (config, error) {
	if len(value.PlansDirs) == 0 && value.PlansDir != "" {
		value.PlansDirs = []string{value.PlansDir}
	}
	if len(value.PlansDirs) == 0 {
		value.PlansDirs = []string{"plan"}
	}
	if err := validatePlanDirs(value.PlansDirs); err != nil {
		return config{}, err
	}
	if err := doc.ValidateLanguage(value.Language); err != nil {
		return config{}, err
	}
	value.Language = doc.NormalizeLanguage(value.Language)
	if err := validateHooks(value.Hooks); err != nil {
		return config{}, err
	}
	if value.Hooks.Timeout == 0 {
		value.Hooks.Timeout = defaultHookTimeout
	}
	return value, nil
}

// planDirs resolves the configured plans directories against the repository root.
func (c config) planDirs(root string) []string {
	paths := make([]string, len(c.PlansDirs))
	for index, directory := range c.PlansDirs {
		paths[index] = filepath.Join(root, directory)
	}
	return paths
}

func planPaths(start string) ([]string, error) {
	value, root, err := loadConfig(start)
	if err != nil {
		return nil, err
	}
	return value.planDirs(root), nil
}

func validatePlanDirs(directories []string) error {
	seen := map[string]bool{}
	for _, directory := range directories {
		if directory == "" || filepath.IsAbs(directory) {
			return errors.New("plans_dirs entries must be non-empty relative paths")
		}
		if seen[directory] {
			return fmt.Errorf("plans_dirs contains duplicate path %q", directory)
		}
		seen[directory] = true
	}
	return nil
}

// ensureGitRepository fails fast when planr is run outside a git repository.
// Completion records are stored as git notes and the done-check reads the
// worktree status, so every plan operation assumes a repository is present.
func ensureGitRepository(start string) error {
	_, err := git.PlainOpenWithOptions(start, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err == nil {
		return nil
	}
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return fmt.Errorf("planr requires a git repository, but %s is not inside one; run `git init` at your project root first", start)
	}
	return fmt.Errorf("cannot open the git repository for %s: %w", start, err)
}
