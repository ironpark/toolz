package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// PromptPlaceholder is substituted into agent.command when it appears there.
// A command that names the placeholder gets the prompt as an argument; one that
// does not gets it on standard input. Both spellings exist because a
// non-interactive agent CLI uses one or the other and mohae cannot guess which.
const PromptPlaceholder = "{{prompt}}"

// customDriver runs an agent that has a non-interactive command line: one
// process per turn, the prompt in, the reply out. It is what lets a tool mohae
// has never heard of be evaluated on the same terms as a built-in driver — and
// what lets the runner be tested without any real agent installed.
//
// Each turn is a fresh process, so the conversation is not carried by the agent
// itself; the prompts a configuration sends have to stand on their own, which
// is the same contract a stateless CLI already imposes on its users.
type customDriver struct {
	command   []string
	env       map[string]string
	workspace string
	executor  processutil.Executor
	onText    func(string)
}

func newCustomDriver(_ context.Context, options Options) (Driver, error) {
	command := options.Command
	if len(command) == 0 {
		// Validate rejects this, so reaching it means the config was built in
		// code rather than loaded; failing here beats exec'ing an empty string.
		return nil, fmt.Errorf("agent.command is required when agent.type is custom-cli")
	}
	return &customDriver{
		command:   command,
		env:       options.Env,
		workspace: options.Workspace,
		executor:  options.executor(),
		onText:    options.OnText,
	}, nil
}

func (d *customDriver) Send(ctx context.Context, prompt string) (Response, error) {
	arguments := make([]string, 0, len(d.command))
	substituted := false
	for _, argument := range d.command {
		if strings.Contains(argument, PromptPlaceholder) {
			argument = strings.ReplaceAll(argument, PromptPlaceholder, prompt)
			substituted = true
		}
		arguments = append(arguments, argument)
	}

	command := d.executor.Command(ctx, arguments, d.workspace, d.env)
	// A backstop for a child that outlives the kill and holds stdout open: the
	// turn's timeout has already expired by then, so waiting further would only
	// stall the run.
	command.WaitDelay = 5 * time.Second
	if !substituted {
		command.Stdin = strings.NewReader(prompt)
	}
	// Stderr is captured rather than forwarded: an agent that logs progress
	// there would otherwise interleave with the dialogue mohae itself prints,
	// and it is worth keeping for the error message when the turn fails.
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Response{}, err
	}
	if err := command.Start(); err != nil {
		return Response{}, fmt.Errorf("agent.command: %w", err)
	}

	// Read on a goroutine so --show-dialogue shows the reply as it is produced
	// rather than after the process exits.
	var text strings.Builder
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		scanner := bufio.NewScanner(stdout)
		// Agent replies routinely exceed bufio's default 64 KiB line.
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			text.WriteString(line)
			text.WriteString("\n")
			if d.onText != nil {
				d.onText(line + "\n")
			}
		}
		// A read error is reported by the process's own exit status; draining
		// what did arrive matters more than the error itself.
		io.Copy(io.Discard, stdout)
	}()
	waitErr := command.Wait()
	wait.Wait()

	response := Response{Text: strings.TrimRight(text.String(), "\n"), Model: d.command[0]}
	if waitErr != nil {
		// The context deadline is the more useful diagnosis when it fired: the
		// process was killed by mohae, not by anything the agent did.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return response, fmt.Errorf("agent.command stopped: %w", ctxErr)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return response, fmt.Errorf("agent.command failed: %w\n%s", waitErr, detail)
		}
		return response, fmt.Errorf("agent.command failed: %w", waitErr)
	}
	return response, nil
}

// Close has nothing to release: a custom CLI's session is one process per turn,
// and each has already exited by the time Send returns.
func (d *customDriver) Close() error { return nil }
