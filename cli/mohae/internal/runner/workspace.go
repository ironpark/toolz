package runner

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ironpark/toolz/cli/mohae/internal/agent"
	"github.com/ironpark/toolz/cli/mohae/internal/container"
	"github.com/ironpark/toolz/cli/mohae/internal/fsutil"
	skillsrc "github.com/ironpark/toolz/cli/mohae/internal/skill"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// Workspace is one trial's isolated copy of the configured source tree.
//
// Root is what the agent works in. Scratch is a sibling directory the verify
// commands run from: grading happens outside the workspace so a check cannot
// leave files behind that would be mistaken for the agent's work. Home is a
// third sibling, and exists so a trial running in a container has a $HOME it
// owns rather than one the image happens to provide.
//
// The two executors are where the trial's commands actually run. They are the
// host unless the configuration asked for a container, and they are the same
// executor only when it asked for the agent to run inside as well: pinning the
// toolchain a trial is built and graded with is useful on its own, and does
// not require the agent's CLI to be in the image.
type Workspace struct {
	Root    string
	Scratch string
	Home    string

	// Skills is what install placed under the agent's skill directory, in the
	// order it did. The trial result carries it forward so a report says which
	// revision of a fetched skill the agent was actually given.
	Skills []SkillInstall

	// exec runs the setup script, the hooks and the verification commands;
	// agent runs the agent under test. They are reached through methods that
	// fall back to the host, so a Workspace built in code rather than by
	// PrepareWorkspace behaves like the uncontainerised trial it describes.
	exec  processutil.Executor
	agent processutil.Executor

	// base holds the directories and is what Cleanup removes. It is also what
	// a container sees mounted, so every path under it maps and nothing else
	// does.
	base string
	// setup holds copies of the scripts the configuration named, which live
	// beside the config file and so are not visible inside a container.
	setup string

	container *container.Container
}

// PrepareWorkspace builds the directory a trial runs in: the source tree copied
// somewhere disposable, the instructions and skills installed, the setup script
// run, and — when asked — a git repository whose first commit is the prepared
// state, so everything the agent did afterwards shows up as a diff.
//
// The configured source is only ever read. Nothing here writes to it, which is
// what lets two runs of the same configuration start from identical state.
func PrepareWorkspace(ctx context.Context, config *Config, agentType string, skills *skillsrc.Resolver) (workspace *Workspace, err error) {
	base, err := os.MkdirTemp("", "mohae-"+fsutil.SanitizeName(config.Name)+"-")
	if err != nil {
		return nil, err
	}
	prepared := &Workspace{
		Root:    filepath.Join(base, "workspace"),
		Scratch: filepath.Join(base, "scratch"),
		Home:    filepath.Join(base, "home"),
		exec:    processutil.Host{},
		agent:   processutil.Host{},
		base:    base,
		setup:   filepath.Join(base, "setup"),
	}
	// One place rather than at each step below: a preparation step added later
	// cannot forget the cleanup call and leave a temporary tree behind.
	defer func() {
		if err != nil {
			prepared.Cleanup()
		}
	}()
	for _, directory := range []string{prepared.Scratch, prepared.Home, prepared.setup} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, err
		}
	}
	if err := copyTreeExcluding(config.Resolve(config.Workspace.Source), prepared.Root, config.Workspace.Exclude); err != nil {
		return nil, fmt.Errorf("copying workspace.source: %w", err)
	}
	if err := prepared.install(ctx, config, agentType, skills); err != nil {
		return nil, err
	}
	// After the files are in place and before anything runs: the container is
	// where the setup script and everything after it happens, and it mounts a
	// directory that has to already hold what the trial provided.
	if config.Container.Enabled() {
		if err := prepared.startContainer(ctx, config); err != nil {
			return nil, err
		}
	}
	if script := config.Workspace.InitScript; script != "" {
		if err := prepared.runInitScript(ctx, config, config.Resolve(script)); err != nil {
			return nil, err
		}
	}
	if config.Workspace.Git {
		// After the setup script, so the baseline commit contains everything the
		// trial provided and nothing the agent produced.
		if err := prepared.initGit(ctx); err != nil {
			return nil, err
		}
	}
	return prepared, nil
}

// install places the documents the agent is meant to find: AGENTS.md at the
// workspace root, and the skills scoped to this agent type under the directory
// that agent reads.
func (w *Workspace) install(ctx context.Context, config *Config, agentType string, skills *skillsrc.Resolver) error {
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
	skillDir, known := agent.SkillDir(agentType)
	for index, entry := range config.Skills {
		if !entry.EnabledFor(agentType) {
			continue
		}
		if !known {
			return fmt.Errorf("skills[%d]: unknown agent type %q", index, agentType)
		}
		installed, err := w.installSkill(ctx, config, entry, skillDir, skills)
		if err != nil {
			return fmt.Errorf("installing skills[%d]: %w", index, err)
		}
		w.Skills = append(w.Skills, installed...)
	}
	return nil
}

// installSkill places one configured entry under the agent's skill directory.
// A local entry is one directory; a remote one may be a repository publishing
// several, and each is installed under its own name.
func (w *Workspace) installSkill(ctx context.Context, config *Config, entry SkillConfig, skillDir string, skills *skillsrc.Resolver) ([]SkillInstall, error) {
	if !entry.Remote() {
		source := config.Resolve(entry.Path)
		if err := w.copySkill(source, skillDir, filepath.Base(source)); err != nil {
			return nil, err
		}
		return []SkillInstall{{Name: filepath.Base(source), Path: entry.Path}}, nil
	}

	parsed, err := skillsrc.ParseSource(entry.Source, entry.Ref, entry.Subpath)
	if err != nil {
		return nil, err
	}
	if skills == nil {
		skills = &skillsrc.Resolver{}
	}
	resolved, err := skills.Resolve(ctx, parsed)
	if err != nil {
		return nil, err
	}
	installs := make([]SkillInstall, 0, len(resolved.Skills))
	for _, found := range resolved.Skills {
		if err := w.copySkill(found.Dir, skillDir, found.Name); err != nil {
			return nil, err
		}
		installs = append(installs, SkillInstall{
			Name:   found.Name,
			Source: parsed.String(),
			Commit: resolved.Commit,
		})
	}
	return installs, nil
}

func (w *Workspace) copySkill(source, skillDir, name string) error {
	target := filepath.Join(w.Root, filepath.FromSlash(skillDir), name)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return copyPath(source, target)
}

// runInitScript runs the setup script inside the copy. Its output is folded
// into the error because a setup failure that only reported an exit status
// would send the caller looking through a workspace that no longer exists.
func (w *Workspace) runInitScript(ctx context.Context, config *Config, script string) error {
	script, err := w.reachable(script)
	if err != nil {
		return fmt.Errorf("workspace.init_script: %w", err)
	}
	// Through runShellStep like the hooks and the verify commands, so a setup
	// script that cannot be started reports the same way one of those would,
	// and with the same variables: setup, work and grading read one
	// environment.
	step := runShellStep(ctx, w.Exec(), shellQuote(w.Exec().Path(script)),
		w.Exec().Path(w.Root), trialEnv(config, w, w.Exec()))
	if !step.Passed {
		return fmt.Errorf("workspace.init_script failed (exit %d)\n%s", step.ExitCode, step.Output)
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

// Close ends the trial's container while leaving its directories in place. It
// is separate from Cleanup because the two have different lifetimes: a failed
// trial keeps its workspace, which is the only record of what the agent did,
// but its container has nothing left to run and would otherwise outlive every
// failing run on the machine. Safe to call more than once.
func (w *Workspace) Close() error {
	if w == nil || w.container == nil {
		return nil
	}
	err := w.container.Remove()
	w.container = nil
	w.exec = processutil.Host{}
	w.agent = processutil.Host{}
	return err
}

// Cleanup removes the trial's directories. It is safe to call more than once.
func (w *Workspace) Cleanup() error {
	if w == nil || w.base == "" {
		return nil
	}
	base := w.base
	w.base = ""
	// The container goes first: it holds the directory open, and anything
	// still running in it would be writing to files as they are removed.
	_ = w.Close()
	return os.RemoveAll(base)
}

// Exec is where the setup script, the hooks and the verification commands run.
func (w *Workspace) Exec() processutil.Executor {
	if w == nil || w.exec == nil {
		return processutil.Host{}
	}
	return w.exec
}

// Agent is where the agent under test runs. It is the same executor as Exec
// only when the configuration asked for container.scope: full.
func (w *Workspace) Agent() processutil.Executor {
	if w == nil || w.agent == nil {
		return processutil.Host{}
	}
	return w.agent
}

// Container describes the container the trial ran in, or an empty string when
// it ran on the host. It is recorded in the report because two runs of one
// configuration on different images are not the same measurement.
func (w *Workspace) Container() string {
	if w == nil || w.container == nil {
		return ""
	}
	return w.container.Image()
}

// startContainer resolves the runtime and starts the trial's container. It is
// reached only when the configuration asked for one, so a missing runtime is
// an error rather than a reason to fall back to the host: a trial that quietly
// ran unsandboxed would be reported as one that ran sandboxed.
func (w *Workspace) startContainer(ctx context.Context, config *Config) error {
	runtime, err := container.Detect(config.Container.Runtime)
	if err != nil {
		return fmt.Errorf("container: %w", err)
	}
	spec := config.ContainerSpec(w.base)
	// A home the trial owns. Both agent CLIs and most package managers write
	// under $HOME, and a container started as the host's user usually has no
	// passwd entry and so no home to write to. The configuration may still
	// override it, which is what a mounted credentials directory needs.
	env := map[string]string{"HOME": container.MountPoint + "/home"}
	maps.Copy(env, spec.Env)
	spec.Env = env

	started, err := container.Start(ctx, runtime, spec)
	if err != nil {
		return err
	}
	w.container = started
	w.exec = started
	if config.Container.AgentInside() {
		w.agent = started
	}
	return nil
}

// reachable returns a path the trial's executor can open. A script named by
// the configuration lives beside the configuration file, which a container
// cannot see, so it is copied into the trial's own directory; on the host it
// is already reachable and is left where it is.
func (w *Workspace) reachable(source string) (string, error) {
	if !w.Exec().Contained() {
		return source, nil
	}
	target := filepath.Join(w.setup, filepath.Base(source))
	if err := copyPath(source, target); err != nil {
		return "", err
	}
	return target, nil
}

// StaleWorkspaceAge is how long a left-behind workspace is kept before a later
// run reclaims it. A failed trial's directory is the only record of what the
// agent did, so it outlives the run by a wide margin — but a trial is bounded
// by limits.timeout_seconds, so nothing this old can still be in use.
const StaleWorkspaceAge = 24 * time.Hour

// PruneStaleWorkspaces removes trial directories older than maxAge from the
// temporary directory and reports how many it removed. Failing trials keep
// their workspace deliberately, and without this they would accumulate there
// for as long as the machine lives. Errors on individual directories are
// ignored: reclaiming disk is never worth failing a run over.
func PruneStaleWorkspaces(maxAge time.Duration) int {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "mohae-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < maxAge {
			continue
		}
		if os.RemoveAll(filepath.Join(os.TempDir(), entry.Name())) == nil {
			removed++
		}
	}
	return removed
}

// copyTree copies a directory recursively, preserving modes so an executable
// fixture stays executable. A .git directory is skipped: the trial makes its
// own baseline commit, and inheriting the source repository's history would
// make the agent's diff unreadable.
func copyTree(source, target string) error {
	return copyTreeExcluding(source, target, nil)
}

// copyTreeExcluding copies a tree while omitting entries matched by a
// source-relative pattern. Matching a directory omits its whole subtree, so a
// fixture can keep evaluation-only files out of the agent's workspace without
// first making a second, filtered fixture.
func copyTreeExcluding(source, target string, exclude []string) error {
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
		if matchesAnyWorkspacePattern(exclude, filepath.ToSlash(relative)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return copyEntry(path, filepath.Join(target, relative), entry)
	})
}

func matchesAnyWorkspacePattern(patterns []string, candidate string) bool {
	for _, pattern := range patterns {
		if matchWorkspacePattern(pattern, candidate) {
			return true
		}
	}
	return false
}

// splitWorkspacePattern breaks a pattern into the segments matchWorkspaceSegments
// walks and validateWorkspacePattern checks, so the two agree on what a
// pattern's segments are. A slashless pattern is basename matching, expressed
// here as an implicit leading **.
func splitWorkspacePattern(pattern string) []string {
	segments := strings.Split(filepath.ToSlash(pattern), "/")
	if len(segments) == 1 {
		return append([]string{"**"}, segments...)
	}
	return segments
}

// matchWorkspacePattern implements the small glob dialect used by excludes
// and artifacts. A slashless pattern follows basename semantics, while ** is a
// whole segment that consumes zero or more path segments.
func matchWorkspacePattern(pattern, candidate string) bool {
	return matchWorkspaceSegments(splitWorkspacePattern(pattern), strings.Split(filepath.ToSlash(candidate), "/"))
}

func matchWorkspaceSegments(pattern, candidate []string) bool {
	if len(pattern) == 0 {
		return len(candidate) == 0
	}
	if pattern[0] == "**" {
		return matchWorkspaceSegments(pattern[1:], candidate) ||
			(len(candidate) > 0 && matchWorkspaceSegments(pattern, candidate[1:]))
	}
	if len(candidate) == 0 {
		return false
	}
	matched, _ := path.Match(pattern[0], candidate[0])
	return matched && matchWorkspaceSegments(pattern[1:], candidate[1:])
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
		return copySymlink(source, target)
	case !info.Mode().IsRegular():
		// Sockets, devices and pipes have no meaning in a copied fixture.
		return nil
	default:
		return copyFile(source, target, info.Mode().Perm())
	}
}

// copySymlink recreates a link rather than following it: a fixture or a
// workspace may point outside the tree being copied on purpose, and resolving
// the link would silently change what gets copied.
func copySymlink(source, target string) error {
	destination, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(destination, target)
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
