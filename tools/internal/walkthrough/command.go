// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Command is one invocation of the plux CLI by one of the walkthrough's
// people.
type Command struct {
	// Actor names the person; their token is the credential of the call.
	Actor string
	// Args are the CLI's arguments, without the program name. Flags come
	// before positional arguments, as the CLI requires.
	Args []string
}

// String renders the command for the transcript: the actor and the arguments,
// never the environment, so the credential cannot appear in it.
func (c Command) String() string { return c.Actor + "$ plux " + strings.Join(c.Args, " ") }

// Result is what a command printed and how it ended.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Commander runs plux CLI commands. A non-zero exit code is a Result, not an
// error; an error means the command could not be run at all.
type Commander interface {
	Run(ctx context.Context, c Command) (Result, error)
}

// StepUpFunc completes a step-up ceremony at url for an actor: in the CI
// run it drives a virtual WebAuthn authenticator in a browser. The CLI waits
// until it has.
type StepUpFunc func(ctx context.Context, actor, url string) error

// ExecCommander runs the real plux binary.
type ExecCommander struct {
	// Binary is the path of the plux executable.
	Binary string
	// Server and Org are given to every command through the environment
	// (PLUX_SERVER, PLUX_ORGANIZATION).
	Server, Org string
	// Initial maps an actor to the access token the CLI uses for them;
	// [ExecCommander.SetToken] adds the people step 1 provisions.
	Initial map[string]string
	// StepUp completes step-up requests; nil leaves them to time out.
	StepUp StepUpFunc
	// Home is the HOME the CLI sees, so that it never reads the real
	// credential store.
	Home string
	// Timeout bounds one command; zero means five minutes.
	Timeout time.Duration
	// CAFile is the PEM file of the CA that signed the server's certificate;
	// the CLI is told to trust it (SSL_CERT_FILE). Empty uses the system roots.
	CAFile string

	mu     sync.Mutex
	tokens map[string]string
}

// Credentials receives the access token of a person once step 1 has made
// them an account.
type Credentials interface {
	SetToken(actor, token string)
}

// SetToken makes token the credential of actor's commands.
func (e *ExecCommander) SetToken(actor, token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tokens == nil {
		e.tokens = map[string]string{}
	}
	e.tokens[actor] = token
}

// token returns the credential of an actor.
func (e *ExecCommander) token(actor string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t, ok := e.tokens[actor]; ok {
		return t, true
	}
	t, ok := e.Initial[actor]
	return t, ok
}

// stepUpLine matches the CLI's request for a step-up and captures its URL.
var stepUpLine = regexp.MustCompile(`Open (\S+) and confirm with your security key`)

// Run executes the command with its actor's token. The token travels in the
// environment of the child only.
func (e *ExecCommander) Run(ctx context.Context, c Command) (Result, error) {
	token, ok := e.token(c.Actor)
	if !ok {
		return Result{}, fmt.Errorf("walkthrough.ExecCommander.Run: no token for actor %q", c.Actor)
	}
	timeout := e.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.Binary, c.Args...) //nolint:gosec // the binary and arguments come from the walkthrough's own steps
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + e.Home,
		"PLUX_SERVER=" + e.Server, "PLUX_ORGANIZATION=" + e.Org, "PLUX_TOKEN=" + token,
	}
	if e.CAFile != "" {
		cmd.Env = append(cmd.Env, "SSL_CERT_FILE="+e.CAFile)
	}
	// A child of the CLI that outlives it must not hold the output pipes open.
	cmd.WaitDelay = 2 * time.Second
	var stdout bytes.Buffer
	watcher := &stepUpWatcher{ctx: ctx, actor: c.Actor, hook: e.StepUp}
	cmd.Stdout, cmd.Stderr = &stdout, watcher
	err := cmd.Run()
	watcher.wait()
	res := Result{Stdout: stdout.String(), Stderr: watcher.text(), ExitCode: 0}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		res.ExitCode = exit.ExitCode()
	default:
		return res, fmt.Errorf("walkthrough.ExecCommander.Run: %w", err)
	}
	if herr := watcher.err(); herr != nil {
		return res, fmt.Errorf("walkthrough.ExecCommander.Run: step-up: %w", herr)
	}
	return res, nil
}

// stepUpWatcher is the CLI's standard error: it keeps the text and, when the
// CLI asks for a step-up, runs the hook once.
type stepUpWatcher struct {
	ctx   context.Context //nolint:containedctx // a writer has no context parameter; this one lives as long as the command
	actor string
	hook  StepUpFunc

	mu    sync.Mutex
	buf   bytes.Buffer
	fired bool
	wg    sync.WaitGroup
	herr  error
}

// Write appends to the text and starts the hook when the request for a
// step-up has been printed.
func (w *stepUpWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if w.hook == nil || w.fired {
		return len(p), nil
	}
	if m := stepUpLine.FindSubmatch(w.buf.Bytes()); m != nil {
		w.fired = true
		w.wg.Add(1)
		go func(url string) {
			defer w.wg.Done()
			if err := w.hook(w.ctx, w.actor, url); err != nil {
				w.mu.Lock()
				w.herr = err
				w.mu.Unlock()
			}
		}(string(m[1]))
	}
	return len(p), nil
}

// wait blocks until a running hook has returned.
func (w *stepUpWatcher) wait() { w.wg.Wait() }

// text returns what the command wrote to standard error.
func (w *stepUpWatcher) text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// err returns the hook's failure, if any.
func (w *stepUpWatcher) err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.herr
}

// ShellHook runs a shell command with the given extra environment, for the
// hooks the maintainer's script provides, and returns its standard output.
func ShellHook(ctx context.Context, command string, env ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // the command is the operator's own flag value
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("walkthrough.ShellHook: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
