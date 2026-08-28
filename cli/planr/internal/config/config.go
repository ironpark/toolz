package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
)

type Config struct {
	PlansDir  string       `yaml:"plans_dir"`
	PlansDirs []string     `yaml:"plans_dirs"`
	Ignore    []string     `yaml:"ignore"`
	Language  string       `yaml:"language"`
	Hooks     hooks.Config `yaml:"hooks"`

	// SkipHooks is set only for the current CLI invocation by --no-hooks. It
	// never comes from .planr.yaml.
	SkipHooks bool `yaml:"-"`

	// Path is populated only when a configuration file was found. It is
	// intentionally not part of the YAML model; the `config` command uses it to
	// explain which file supplied the effective values.
	Path string `yaml:"-"`
}

func Load(start string) (Config, string, error) {
	location, err := Discover(start)
	if err != nil {
		return Config{}, "", err
	}
	if location.Path == "" {
		return Default(), location.BaseRoot, nil
	}
	value, err := ParseFile(location.Path)
	if err != nil {
		return Config{}, "", err
	}
	value.Path = location.Path
	return value, location.BaseRoot, nil
}

// Location describes both the root against which relative settings are
// resolved and the file selected by the upward search. When the starting path
// is inside a worktree, the search stops at the worktree root: parent
// directories above it must not unexpectedly contribute another .planr.yaml.
type Location struct {
	BaseRoot string
	Path     string
}

func Discover(start string) (Location, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return Location{}, err
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
			return Location{BaseRoot: baseRoot, Path: path}, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			// Preserve the path so ParseFile can return the useful read
			// error, instead of silently treating an inaccessible config as absent.
			return Location{BaseRoot: baseRoot, Path: path}, nil
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
	return Location{BaseRoot: baseRoot}, nil
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

func Default() Config {
	return Config{PlansDirs: []string{"plan"}, Language: doc.DefaultLanguage, Hooks: hooks.Config{Timeout: hooks.DefaultTimeout}}
}

func ParseFile(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var value Config
	if err := yaml.UnmarshalWithOptions(contents, &value, yaml.Strict()); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	value, err = Normalize(value)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

func Normalize(value Config) (Config, error) {
	if len(value.PlansDirs) == 0 && value.PlansDir != "" {
		value.PlansDirs = []string{value.PlansDir}
	}
	if len(value.PlansDirs) == 0 {
		value.PlansDirs = []string{"plan"}
	}
	if err := ValidatePlanDirs(value.PlansDirs); err != nil {
		return Config{}, err
	}
	if err := doc.ValidateLanguage(value.Language); err != nil {
		return Config{}, err
	}
	value.Language = doc.NormalizeLanguage(value.Language)
	if err := hooks.Validate(value.Hooks); err != nil {
		return Config{}, err
	}
	if value.Hooks.Timeout == 0 {
		value.Hooks.Timeout = hooks.DefaultTimeout
	}
	return value, nil
}

// PlanDirs resolves the configured plans directories against the repository root.
func (c Config) PlanDirs(root string) []string {
	paths := make([]string, len(c.PlansDirs))
	for index, directory := range c.PlansDirs {
		paths[index] = filepath.Join(root, directory)
	}
	return paths
}

func PlanPaths(start string) ([]string, error) {
	value, root, err := Load(start)
	if err != nil {
		return nil, err
	}
	return value.PlanDirs(root), nil
}

func ValidatePlanDirs(directories []string) error {
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
