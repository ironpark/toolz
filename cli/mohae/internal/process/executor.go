package process

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Executor is where a trial's commands run. Everything mohae starts on a
// trial's behalf — the setup script, the hooks, the verification commands and,
// when the configuration asks for it, the agent itself — is built through one
// of these, so what a trial can reach is decided in a single place instead of
// at each call site.
//
// Paths are the reason the interface has more than one method. A containerised
// trial sees its workspace at a different path from the one mohae copied files
// to, and a command told the host's path would work in a directory that does
// not exist. Path is the only translation, and it is explicit: dir and any
// path inside env are already in the executor's namespace by the time Command
// receives them.
type Executor interface {
	// Command builds the process for argv, run from dir with env laid over
	// whatever base environment the executor provides.
	Command(ctx context.Context, argv []string, dir string, env map[string]string) *exec.Cmd
	// Path maps a path on this host to where a command run by this executor
	// sees it. A path the executor cannot reach is returned unchanged: it is
	// better for a command to fail on a missing file than to silently read a
	// different one.
	Path(host string) string
	// Contained reports whether commands run anywhere other than this host. A
	// driver reads it to decide whether looking for the agent CLI on the local
	// filesystem means anything.
	Contained() bool
}

// Host runs commands as child processes of this one. It is the executor a
// trial gets when no container is configured, and the zero value is ready to
// use.
type Host struct{}

// Command builds a local subprocess in its own process group, so cancelling
// the context reaches whatever the command started.
func (Host) Command(ctx context.Context, argv []string, dir string, env map[string]string) *exec.Cmd {
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = dir
	command.Env = Env(env)
	Isolate(command)
	return command
}

// Path is the identity: a host command already sees the host's filesystem.
func (Host) Path(host string) string { return host }

// Contained is false: this is the host.
func (Host) Contained() bool { return false }

// Shell builds the `sh -c` form hooks, verification and the setup script all
// use, so the shell mohae grades with is chosen once.
func Shell(ctx context.Context, executor Executor, text, dir string, env map[string]string) *exec.Cmd {
	return executor.Command(ctx, []string{"sh", "-c", text}, dir, env)
}

// Overlay reduces a fully built environment to the variables that differ from
// this process's own. An SDK that returns os.Environ() plus its own additions
// is the caller this exists for: only the additions are meaningful somewhere
// that is not this host, and forwarding the rest would carry the host's PATH
// and HOME into a container that has its own.
func Overlay(env []string) map[string]string {
	current := currentEnv()
	overlay := map[string]string{}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if existing, inherited := current[key]; !inherited || existing != value {
			overlay[key] = value
		}
	}
	return overlay
}

// currentEnv is this process's own environment as a map, built once: it does
// not change during a run, and Overlay is called for every agent subprocess.
var currentEnv = sync.OnceValue(func() map[string]string {
	current := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			current[key] = value
		}
	}
	return current
})
