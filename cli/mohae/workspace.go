package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Workspace is one trial's isolated copy of the configured source tree.
//
// Root is what the agent works in. Scratch is a sibling directory the verify
// commands run from: grading happens outside the workspace so a check cannot
// leave files behind that would be mistaken for the agent's work.
type Workspace struct {
	Root    string
	Scratch string

	// base holds both directories and is what Cleanup removes.
	base string
}

// skillDirectories say where each agent looks for the skills installed into a
// workspace. A skill dropped somewhere the agent never reads would be a trial
// that silently measured the agent without it.
var skillDirectories = map[string]string{
	"claude-code": ".claude/skills",
	"codex":       ".codex/skills",
	"custom-cli":  ".agent/skills",
}

// PrepareWorkspace builds the directory a trial runs in: the source tree copied
// somewhere disposable, the instructions and skills installed, the setup script
// run, and — when asked — a git repository whose first commit is the prepared
// state, so everything the agent did afterwards shows up as a diff.
//
// The configured source is only ever read. Nothing here writes to it, which is
// what lets two runs of the same configuration start from identical state.
func PrepareWorkspace(ctx context.Context, config *Config, agentType string) (*Workspace, error) {
	base, err := os.MkdirTemp("", "mohae-"+sanitizeName(config.Name)+"-")
	if err != nil {
		return nil, err
	}
	workspace := &Workspace{
		Root:    filepath.Join(base, "workspace"),
		Scratch: filepath.Join(base, "scratch"),
		base:    base,
	}
	if err := os.MkdirAll(workspace.Scratch, 0o755); err != nil {
		workspace.Cleanup()
		return nil, err
	}
	if err := copyTree(config.Resolve(config.Workspace.Source), workspace.Root); err != nil {
		workspace.Cleanup()
		return nil, fmt.Errorf("copying workspace.source: %w", err)
	}
	if err := workspace.install(config, agentType); err != nil {
		workspace.Cleanup()
		return nil, err
	}
	if script := config.Workspace.InitScript; script != "" {
		if err := workspace.runInitScript(ctx, config.Resolve(script)); err != nil {
			workspace.Cleanup()
			return nil, err
		}
	}
	if config.Workspace.Git {
		// After the setup script, so the baseline commit contains everything the
		// trial provided and nothing the agent produced.
		if err := workspace.initGit(ctx); err != nil {
			workspace.Cleanup()
			return nil, err
		}
	}
	return workspace, nil
}

// install places the documents the agent is meant to find: AGENTS.md at the
// workspace root, and the skills scoped to this agent type under the directory
// that agent reads.
func (w *Workspace) install(config *Config, agentType string) error {
	if source := config.Workspace.AgentMD; source != "" {
		data, err := os.ReadFile(config.Resolve(source))
		if err != nil {
			return fmt.Errorf("reading workspace.agent_md: %w", err)
		}
		// Installed under the name the agent expects, whatever the source file
		// was called, so one document can be shared by every configuration.
		if err := os.WriteFile(filepath.Join(w.Root, "AGENTS.md"), data, 0o644); err != nil {
			return err
		}
	}
	directory, known := skillDirectories[agentType]
	for index, skill := range config.Skills {
		if !skill.EnabledFor(agentType) {
			continue
		}
		if !known {
			return fmt.Errorf("skills[%d]: no skill directory is known for agent type %q", index, agentType)
		}
		source := config.Resolve(skill.Path)
		target := filepath.Join(w.Root, filepath.FromSlash(directory), filepath.Base(source))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyPath(source, target); err != nil {
			return fmt.Errorf("installing skills[%d]: %w", index, err)
		}
	}
	return nil
}

// runInitScript runs the setup script inside the copy. Its output is folded
// into the error because a setup failure that only reported an exit status
// would send the caller looking through a workspace that no longer exists.
func (w *Workspace) runInitScript(ctx context.Context, script string) error {
	command := exec.CommandContext(ctx, "sh", "-c", shellQuote(script))
	command.Dir = w.Root
	command.Env = append(os.Environ(), "MOHAE_WORKSPACE="+w.Root)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("workspace.init_script failed: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// initGit makes the prepared state the repository's first commit. The identity
// is set on the repository itself: a machine with no global git config would
// otherwise fail to commit, and a trial should not depend on how the host is
// configured.
func (w *Workspace) initGit(ctx context.Context) error {
	commands := [][]string{
		{"git", "init", "--quiet"},
		{"git", "config", "user.email", "mohae@localhost"},
		{"git", "config", "user.name", "mohae"},
		{"git", "add", "-A"},
		{"git", "commit", "--quiet", "--allow-empty", "-m", "mohae: trial baseline"},
	}
	for _, arguments := range commands {
		command := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
		command.Dir = w.Root
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("workspace.git: %s failed: %w\n%s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// Cleanup removes the trial's directories. It is safe to call more than once.
func (w *Workspace) Cleanup() error {
	if w == nil || w.base == "" {
		return nil
	}
	base := w.base
	w.base = ""
	return os.RemoveAll(base)
}

// copyTree copies a directory recursively, preserving modes so an executable
// fixture stays executable. A .git directory is skipped: the trial makes its
// own baseline commit, and inheriting the source repository's history would
// make the agent's diff unreadable.
func copyTree(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", source)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		return copyEntry(path, filepath.Join(target, relative), entry)
	})
}

func copyEntry(source, target string, entry os.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	switch {
	case entry.IsDir():
		return os.MkdirAll(target, info.Mode().Perm())
	case info.Mode()&os.ModeSymlink != 0:
		// Recreated as a link rather than followed: a fixture may point outside
		// itself on purpose, and resolving the link would silently change what
		// the agent sees.
		destination, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(destination, target)
	case !info.Mode().IsRegular():
		// Sockets, devices and pipes have no meaning in a copied fixture.
		return nil
	default:
		return copyFile(source, target, info.Mode().Perm())
	}
}

// copyPath copies either a file or a directory, which is what a skill may be.
func copyPath(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyTree(source, target)
	}
	return copyFile(source, target, info.Mode().Perm())
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// shellQuote wraps a path for `sh -c` so a script in a directory with a space
// in its name still runs.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// sanitizeName keeps a config name usable as a directory prefix.
func sanitizeName(name string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	if mapped == "" {
		return "trial"
	}
	return mapped
}
