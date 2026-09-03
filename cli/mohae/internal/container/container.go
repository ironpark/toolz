package container

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// MountPoint is where a trial's own directory appears inside the container.
// It is fixed rather than derived from the host path, so two runs of the same
// configuration give the agent the same paths even though the host directory
// is a fresh temporary one each time — a trial whose transcript quoted
// /var/folders/xyz would not be comparable with the next one.
const MountPoint = "/mohae"

// User values with a meaning of their own; anything else is passed to the
// runtime as written.
const (
	UserHost = "host"
	UserRoot = "root"
)

// BaseLabel records the trial directory a container was started for.
const BaseLabel = "mohae.base"

// killGrace bounds both the in-container kill and the wait for a client that
// ignored its own cancellation. The command it belongs to has already run out
// of time by the time either is reached, so waiting longer only delays the
// result the caller is owed.
const killGrace = 5 * time.Second

// Mount is one host directory made visible inside the container.
type Mount struct {
	Host   string
	Target string
	// ReadOnly is what a mount carrying credentials should be: the agent
	// needs to log in, not to rewrite the host's configuration.
	ReadOnly bool
	// Relabel asks Podman to apply a private SELinux label to the host
	// directory. It is what makes a bind mount readable on an enforcing host,
	// and it changes labels on the host, so it is only ever set for
	// directories mohae created itself.
	Relabel bool
}

// Spec is the container one trial runs in.
type Spec struct {
	// Image is what to run. Build takes precedence when both are set, which
	// configuration validation prevents.
	Image string
	// Build is a directory containing a Dockerfile, built before the container
	// starts.
	Build string
	// Base is the trial's own directory on the host, mounted at MountPoint.
	Base string
	// Network is passed to the runtime as written. "none" is what a trial
	// wanting reproducible verification asks for.
	Network string
	// User selects who the trial runs as. See UserHost and UserRoot.
	User string
	// Mounts are additional host directories the configuration asked for.
	Mounts []Mount
	// Env is set on the container, and so is seen by every command execed
	// into it.
	Env map[string]string
}

// Container is one trial's running container. It implements
// process.Executor, which is how the runner reaches it: nothing outside this
// package needs to know a trial is containerised.
type Container struct {
	runtime *Runtime
	id      string
	base    string
	image   string

	execs   atomic.Uint64
	removed atomic.Bool
}

// Start builds the image if the configuration named a Dockerfile, then starts
// a container that stays up for the trial's lifetime. Nothing is run in it
// here: the container is a place to run things, and what runs is the setup
// script, the agent, the hooks and the verification commands, in that order.
func Start(ctx context.Context, runtime *Runtime, spec Spec) (*Container, error) {
	image := spec.Image
	if spec.Build != "" {
		built, err := runtime.run(ctx, "build", "--quiet", spec.Build)
		if err != nil {
			return nil, fmt.Errorf("container.build: %w", err)
		}
		image = lastLine(built)
		if image == "" {
			return nil, fmt.Errorf("container.build: %s build printed no image id", runtime.Name)
		}
	}
	args, err := spec.runArgs(image, runtime.Podman())
	if err != nil {
		return nil, err
	}
	id, err := runtime.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("container.image %s: %w", image, err)
	}
	return &Container{runtime: runtime, id: lastLine(id), base: spec.Base, image: image}, nil
}

// idleScript keeps the container alive while mohae execs into it. The image's
// own entrypoint is replaced because what an image runs by default is its
// application, and a trial needs a shell it can come back to between steps.
const idleScript = "while :; do sleep 3600; done"

// runArgs builds the `run` command line. It is separated from Start, and takes
// the runtime's identity as a parameter rather than reading it, so what mohae
// asks a runtime for can be tested on a machine that has neither installed.
func (s Spec) runArgs(image string, podman bool) ([]string, error) {
	args := []string{"run", "--detach", "--init"}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	user, err := s.userArgs(podman)
	if err != nil {
		return nil, err
	}
	args = append(args, user...)
	// Labelled with the directory it serves, so a container orphaned by a
	// mohae that was killed can be told apart from every other container on
	// the machine and reclaimed by a later run.
	args = append(args, "--label", BaseLabel+"="+s.Base)
	args = append(args, "--volume", mountArg(Mount{Host: s.Base, Target: MountPoint, Relabel: true}, podman))
	for _, mount := range s.Mounts {
		args = append(args, "--volume", mountArg(mount, podman))
	}
	// Sorted so two runs of one configuration produce the same command line;
	// a map's order would otherwise be the only difference between them.
	for _, key := range slices.Sorted(maps.Keys(s.Env)) {
		args = append(args, "--env", key+"="+s.Env[key])
	}
	args = append(args, "--workdir", MountPoint, "--entrypoint", "sh", image, "-c", idleScript)
	return args, nil
}

// userArgs decides who the trial runs as. The default matters more than it
// looks: the workspace is a bind mount, so whatever writes to it inside the
// container owns the files on the host, and a run as root leaves a workspace
// the user who started mohae cannot delete.
func (s Spec) userArgs(podman bool) ([]string, error) {
	switch s.User {
	case "", UserHost:
		uid, gid := os.Getuid(), os.Getgid()
		if uid < 0 || gid < 0 {
			// No host ids to map, which is Windows. The runtime's own default
			// is then the only sensible answer.
			return nil, nil
		}
		args := []string{"--user", fmt.Sprintf("%d:%d", uid, gid)}
		if podman {
			// Rootless Podman remaps ids by default, so the invoking user
			// would appear inside as someone else and the files it wrote
			// would come back owned by a subordinate id. keep-id maps the
			// user onto itself, which is what a bind mount needs.
			args = append(args, "--userns=keep-id")
		}
		return args, nil
	case UserRoot:
		return []string{"--user", "0:0"}, nil
	default:
		return []string{"--user", s.User}, nil
	}
}

func mountArg(mount Mount, podman bool) string {
	options := []string{}
	if mount.ReadOnly {
		options = append(options, "ro")
	}
	if mount.Relabel && podman {
		options = append(options, "Z")
	}
	argument := mount.Host + ":" + mount.Target
	if len(options) > 0 {
		argument += ":" + strings.Join(options, ",")
	}
	return argument
}

// execIDVar marks the processes belonging to one command. It is an environment
// variable rather than a recorded pid because children inherit it: cancelling
// a step has to reach the work it started, not only the shell that started it.
const execIDVar = "MOHAE_EXEC_ID"

// killScript kills every process in the container carrying the marker it is
// given. It walks /proc rather than using pkill, which images routinely do not
// have, and always exits zero: a step whose processes are already gone has
// nothing to report.
const killScript = `for entry in /proc/[0-9]*; do
  if tr '\0' '\n' < "$entry/environ" 2>/dev/null | grep -qxF "$1"; then
    kill -9 "${entry#/proc/}" 2>/dev/null || true
  fi
done
exit 0`

// Command builds a process that runs argv inside the container. dir and the
// paths in env are container paths; map them with Path first.
func (c *Container) Command(ctx context.Context, argv []string, dir string, env map[string]string) *exec.Cmd {
	marker := strconv.FormatUint(c.execs.Add(1), 10)
	args := []string{"exec", "--interactive", "--env", execIDVar + "=" + marker}
	if dir != "" {
		args = append(args, "--workdir", dir)
	}
	for _, key := range slices.Sorted(maps.Keys(env)) {
		args = append(args, "--env", key+"="+env[key])
	}
	args = append(args, c.id)
	args = append(args, argv...)

	command := exec.CommandContext(ctx, c.runtime.Path, args...)
	// The client's own environment, not the trial's: it needs DOCKER_HOST and
	// the like to find the daemon, while the trial's variables travel as
	// --env so they reach the process inside rather than the client.
	command.Env = os.Environ()
	processutil.Isolate(command)
	// Set after Isolate, which would otherwise only kill the client and leave
	// the process it started running inside the container — for a trial whose
	// agent turn timed out, that is a process still writing to the workspace
	// mohae is about to grade.
	command.Cancel = func() error {
		c.killExec(marker)
		if command.Process == nil {
			return nil
		}
		return command.Process.Kill()
	}
	command.WaitDelay = killGrace
	return command
}

func (c *Container) killExec(marker string) {
	// Detached from the caller's context, which is already cancelled by the
	// time this runs.
	ctx, cancel := context.WithTimeout(context.Background(), killGrace)
	defer cancel()
	command := exec.CommandContext(ctx, c.runtime.Path,
		"exec", c.id, "sh", "-c", killScript, "sh", execIDVar+"="+marker)
	_ = command.Run()
}

// Path maps a host path to where the container sees it. A path outside the
// trial's own directory is returned unchanged: the container cannot see it,
// and a command failing on a missing file is a better outcome than one
// quietly reading a different file that happens to exist at the mapped name.
func (c *Container) Path(host string) string {
	relative, err := filepath.Rel(c.base, host)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return host
	}
	if relative == "." {
		return MountPoint
	}
	return path.Join(MountPoint, filepath.ToSlash(relative))
}

// Contained is true: this is the whole point of the type.
func (c *Container) Contained() bool { return true }

// ID is the container the trial ran in, for the report.
func (c *Container) ID() string { return c.id }

// Image is what the container was started from — the built image's id when
// the configuration named a Dockerfile, so a report says what actually ran
// rather than repeating the path it was built from.
func (c *Container) Image() string { return c.image }

// Remove kills the container and everything still running in it. It is safe to
// call more than once, and runs on its own context because it is reached from
// cleanup after the trial's own deadline has passed.
func (c *Container) Remove() error {
	if c == nil || !c.removed.CompareAndSwap(false, true) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), killGrace)
	defer cancel()
	_, err := c.runtime.run(ctx, "rm", "--force", "--volumes", c.id)
	return err
}

// lastLine is what a runtime prints as its answer. Both runtimes may write
// progress before it, so the id is the last line rather than the whole output.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// PruneStale removes containers left behind by a mohae that did not get to
// clean up after itself — one that was killed, or that crashed. A container is
// stale when the trial directory it was started for is gone: that directory is
// removed last, and a container whose directory still exists may belong to a
// run happening right now.
//
// It reports how many it removed. Every failure is ignored, including having
// no runtime at all: reclaiming a leaked container is never worth failing a
// run over, and a machine with no runtime cannot have leaked one.
func PruneStale() int {
	runtime, err := Detect(Auto)
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), pruneGrace)
	defer cancel()
	listed, err := runtime.run(ctx, "ps", "--all", "--quiet",
		"--filter", "label="+BaseLabel, "--format", "{{.ID}} {{.Labels."+BaseLabel+"}}")
	if err != nil || strings.TrimSpace(listed) == "" {
		return 0
	}
	removed := 0
	for _, line := range strings.Split(listed, "\n") {
		id, base, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || base == "" {
			continue
		}
		if _, err := os.Stat(base); err == nil {
			continue
		}
		if _, err := runtime.run(ctx, "rm", "--force", "--volumes", id); err == nil {
			removed++
		}
	}
	return removed
}

// pruneGrace bounds the whole sweep. It runs before a benchmark rather than as
// part of one, and a runtime that is not answering should delay the run by a
// few seconds at most.
const pruneGrace = 20 * time.Second
