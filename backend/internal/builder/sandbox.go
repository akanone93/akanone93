// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ErrSandboxUnavailable is returned by a sandbox that cannot isolate a child,
// for example because the kernel refuses user or network namespaces
// (FN-016). The builder never retries without the sandbox; running unsandboxed
// is an explicit, separate choice ([NoSandbox]).
var ErrSandboxUnavailable = errors.New("builder: sandbox unavailable")

// waitDelay bounds how long Run waits for the output pipes after the process
// group was killed.
const waitDelay = 2 * time.Second

// Limits bound one child process (FN-016). A zero field means no limit.
type Limits struct {
	// CPUTime is the CPU time one process may use (RLIMIT_CPU); it applies to
	// each process of the build separately.
	CPUTime time.Duration
	// Memory is the address space one process may map, in bytes (RLIMIT_AS); it
	// applies to each process of the build separately.
	Memory uint64
	// Wall is the elapsed time of the whole process group.
	Wall time.Duration
	// OutputBytes bounds standard output and standard error separately; a child
	// that writes more is killed.
	OutputBytes int64
}

// Command is one child process to run.
type Command struct {
	// Path is the executable.
	Path string
	// Args are the arguments after the program name.
	Args []string
	// Dir is the working directory.
	Dir string
	// Env is the complete environment of the child; nothing is inherited.
	Env []string
	// Limits bound the child.
	Limits Limits
	// Stdin, when set, is the child's standard input; otherwise it reads
	// nothing. A language server session feeds the child through it (FN-033).
	Stdin io.Reader
	// Stdout, when set, receives the child's standard output as it is
	// written, and Result.Stdout stays empty; the caller bounds what it
	// accepts. Standard error is always captured and bounded.
	Stdout io.Writer
}

// Result is the outcome of a command that ran to an end, whatever its exit
// status.
type Result struct {
	// ExitCode is the exit status, or -1 when a signal ended the process.
	ExitCode int
	// Signal is the number of the signal that ended the process, or 0.
	Signal int
	// Stdout is the captured standard output, cut at Limits.OutputBytes.
	Stdout []byte
	// Stderr is the captured standard error, cut at Limits.OutputBytes.
	Stderr []byte
	// TimedOut is true when Limits.Wall elapsed and the group was killed.
	TimedOut bool
	// OutputExceeded is true when the child wrote more than Limits.OutputBytes
	// and was killed.
	OutputExceeded bool
}

// Sandbox runs a child process with the isolation and limits of its Command.
// Implementations are safe for concurrent use.
type Sandbox interface {
	// Run starts the command, waits for it and returns its result. An error is
	// returned when the command could not run at all, including
	// ErrSandboxUnavailable; a non-zero exit status is not an error.
	Run(ctx context.Context, cmd Command) (Result, error)
}

// NoSandbox runs children without namespaces or resource limits. It exists for
// `plux fn build` on a developer machine and for the development setting
// `builder.sandbox: none`; the caller logs the distinct warning (FN-016,
// security defaults). The wall-time and output limits still apply. It is safe
// for concurrent use.
type NoSandbox struct{}

// Run implements Sandbox.
func (NoSandbox) Run(ctx context.Context, cmd Command) (Result, error) {
	return runChild(ctx, cmd, childHooks{})
}

// childHooks adapt the shared runner to a sandbox. Every field is optional.
type childHooks struct {
	// prepare sets the process attributes before the start.
	prepare func(c *exec.Cmd)
	// afterStart runs once the child exists, for example to set its limits.
	afterStart func(c *exec.Cmd, limits Limits) error
	// classifyStart interprets an error from starting the child.
	classifyStart func(err error) error
}

// runChild is the runner both sandboxes share.
func runChild(ctx context.Context, cmd Command, h childHooks) (Result, error) {
	if cmd.Path == "" {
		return Result{}, errors.New("builder.Run: empty command path")
	}
	wallCtx, cancelWall := withWall(ctx, cmd.Limits.Wall)
	defer cancelWall()
	runCtx, cancelRun := context.WithCancel(wallCtx)
	defer cancelRun()

	stdout := newBoundedBuffer(cmd.Limits.OutputBytes, cancelRun)
	stderr := newBoundedBuffer(cmd.Limits.OutputBytes, cancelRun)
	var stdoutSink io.Writer = stdout
	if cmd.Stdout != nil {
		stdoutSink = cmd.Stdout
	}
	c := exec.CommandContext(runCtx, cmd.Path, cmd.Args...) //nolint:gosec // G204: the builder constructs every command itself.
	c.Dir = cmd.Dir
	c.Env = append([]string{}, cmd.Env...)
	c.Stdin, c.Stdout, c.Stderr = cmd.Stdin, stdoutSink, stderr
	c.WaitDelay = waitDelay
	configureGroup(c)
	if h.prepare != nil {
		h.prepare(c)
	}

	// Pdeathsig is tied to the thread that starts the child; keep it alive
	// until the child is gone.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := c.Start(); err != nil {
		if h.classifyStart != nil {
			err = h.classifyStart(err)
		}
		return Result{}, fmt.Errorf("builder.Run: start %s: %w", cmd.Path, err)
	}
	if h.afterStart != nil {
		if err := h.afterStart(c, cmd.Limits); err != nil {
			cancelRun()
			_ = c.Wait()
			return Result{}, fmt.Errorf("builder.Run: limits for %s: %w", cmd.Path, err)
		}
	}
	waitErr := c.Wait()

	res := Result{
		ExitCode:       c.ProcessState.ExitCode(),
		Signal:         exitSignal(c.ProcessState),
		Stdout:         stdout.Bytes(),
		Stderr:         stderr.Bytes(),
		OutputExceeded: stdout.exceeded() || stderr.exceeded(),
	}
	if err := ctx.Err(); err != nil {
		return res, fmt.Errorf("builder.Run: %w", err)
	}
	res.TimedOut = errors.Is(wallCtx.Err(), context.DeadlineExceeded) && !res.OutputExceeded
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) && !res.TimedOut && !res.OutputExceeded {
		return res, fmt.Errorf("builder.Run: wait for %s: %w", cmd.Path, waitErr)
	}
	return res, nil
}

// withWall derives a context that ends after wall; zero means no limit.
func withWall(ctx context.Context, wall time.Duration) (context.Context, context.CancelFunc) {
	if wall <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, wall)
}

// boundedBuffer keeps the first limit bytes written to it and calls onFull
// once when more arrive. It is safe for concurrent use.
type boundedBuffer struct {
	mu     sync.Mutex // guards buf
	buf    bytes.Buffer
	limit  int64
	onFull func()
	full   atomic.Bool
}

// newBoundedBuffer returns a buffer cut at limit bytes; a limit of zero or less
// keeps everything.
func newBoundedBuffer(limit int64, onFull func()) *boundedBuffer {
	return &boundedBuffer{limit: limit, onFull: onFull}
}

// Write keeps what fits and always reports the whole slice as written, so the
// child is not blocked on a full pipe before it is killed.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		b.buf.Write(p)
		return len(p), nil
	}
	room := b.limit - int64(b.buf.Len())
	if int64(len(p)) <= room {
		b.buf.Write(p)
		return len(p), nil
	}
	if room > 0 {
		b.buf.Write(p[:room])
	}
	if b.full.CompareAndSwap(false, true) && b.onFull != nil {
		b.onFull()
	}
	return len(p), nil
}

// Bytes returns a copy of what was kept.
func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

// exceeded reports whether more than the limit was written.
func (b *boundedBuffer) exceeded() bool { return b.full.Load() }
