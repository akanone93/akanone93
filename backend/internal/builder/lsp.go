// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The errors of [LanguageSession.Send], for a caller that maps them to a status.
var (
	// ErrLanguageSequence is returned for a message whose sequence number is
	// not the next one.
	ErrLanguageSequence = errors.New("builder: the message is out of sequence")
	// ErrLanguageEnded is returned for a message sent after the session ended.
	ErrLanguageEnded = errors.New("builder: the language-server session has ended")
	// ErrLanguageMessageSize is returned for a message over lsp.messageBytes.
	ErrLanguageMessageSize = errors.New("builder: the message exceeds lsp.messageBytes")
)

// virtualRoot is the workspace root a client addresses documents under. The
// session maps it to the sandbox's scratch directory in both directions, so
// no host path reaches the browser and no document outside the workspace can
// be named (FN-033, ADR-0099).
const virtualRoot = "/workspace"

// stderrTail is how much of a failed language server's standard error the end
// of a session reports.
const stderrTail = 2048

// messageQueue is how many server messages wait for the consumer before the
// language server's output is held back.
const messageQueue = 256

// LanguageOptions configure one language-server session (FN-033).
type LanguageOptions struct {
	// Server is the language server executable, rust-analyzer.
	Server string
	// Args are its arguments.
	Args []string
	// Env is added to the environment of the child; it never carries a
	// credential. The builder's own environment is not inherited.
	Env []string
	// Wall is the session time (lsp.sessionMinutes); the sandbox kills the
	// language server when it passes. It must be positive.
	Wall time.Duration
	// Memory is the address space the language server may map, in bytes
	// (function.buildMemory); zero means no limit.
	Memory uint64
	// MessageBytes bounds one message in either direction (lsp.messageBytes).
	// It must be positive.
	MessageBytes int64
}

// LanguageEnd says why a session ended.
type LanguageEnd struct {
	// Reason is "client" when the caller closed the session, "time" when
	// the session time passed, "exited" when the language server ended on
	// its own with status 0 and "failed" otherwise.
	Reason string
	// Detail is a short diagnostic without host paths.
	Detail string
}

// LanguageSession is one language server running in the build sandbox for one
// editor session. It is safe for concurrent use, except that Send calls of one
// session are ordered by their sequence numbers.
type LanguageSession struct {
	root     string // the real workspace directory
	rootURI  string // the virtual crate root the client addresses
	policy   *languagePolicy
	maxBytes int64

	ctx      context.Context
	cancel   context.CancelFunc
	stdin    *os.File
	messages chan []byte
	done     chan struct{}

	sendMu  sync.Mutex // orders Send and guards nextSeq
	nextSeq uint64

	mu     sync.Mutex // guards closed, end and messages' close
	closed bool
	end    LanguageEnd
}

// OpenLanguage stages the function's workspace in a scratch directory and
// starts the language server in the sandbox with build scripts and procedural
// macros off (FN-033, ADR-0099). The server runs with no network, no database
// or signing credential and the CPU, memory and time limits of the options.
// The session lives until ctx ends, Close is called, the session time passes
// or the process exits. [ErrSandboxUnavailable] is returned when the kernel
// refuses the sandbox; the session never runs without it unless the builder
// was configured with [NoSandbox].
func (b *Builder) OpenLanguage(ctx context.Context, src FunctionSource, o LanguageOptions) (*LanguageSession, error) {
	if o.Server == "" || o.Wall <= 0 || o.MessageBytes <= 0 {
		return nil, errors.New("builder.OpenLanguage: the server, the session time and the message limit are required (LIM-001)")
	}
	server, err := exec.LookPath(o.Server)
	if err != nil {
		return nil, fmt.Errorf("builder.OpenLanguage: language server %q: %w", o.Server, err)
	}
	funcs, err := prepareSources([]FunctionSource{src})
	if err != nil {
		return nil, err
	}
	r, cleanup, err := b.newRun()
	if err != nil {
		return nil, err
	}
	if _, err := stageWorkspace(r.paths.Workspace, b.sdk, funcs, b.cfg.SourceBytes); err != nil {
		cleanup()
		return nil, err
	}
	if err := scrubWorkspace(r.paths.Workspace); err != nil {
		cleanup()
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	// An operating-system pipe, not an io.Pipe: the child gets the read end
	// itself, so no goroutine of os/exec is left blocked on it when the
	// process ends.
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		cleanup()
		return nil, fmt.Errorf("builder.OpenLanguage: %w", err)
	}
	s := &LanguageSession{
		root:     r.paths.Workspace,
		rootURI:  "file://" + virtualRoot + "/" + funcs[0].dir() + "/",
		maxBytes: o.MessageBytes,
		ctx:      runCtx, cancel: cancel, stdin: pw,
		messages: make(chan []byte, messageQueue), done: make(chan struct{}), nextSeq: 1,
	}
	s.policy = newLanguagePolicy(r.paths.Workspace, s.rootURI)
	fw := &frameReader{limit: o.MessageBytes, deliver: func(raw []byte) error { return s.fromServer(runCtx, raw) }, onBad: cancel}
	cmd := Command{
		Path: server, Args: o.Args, Dir: r.paths.Workspace,
		Env:    append(append([]string{}, r.env...), o.Env...),
		Limits: Limits{CPUTime: o.Wall, Memory: o.Memory, Wall: o.Wall, OutputBytes: stderrCapture},
		Stdin:  pr, Stdout: fw,
	}
	go func() {
		defer cleanup()
		res, runErr := b.cfg.Sandbox.Run(runCtx, cmd)
		_ = pr.Close()
		_ = pw.Close()
		s.finish(runCtx, res, runErr, fw, r)
	}()
	return s, nil
}

// stderrCapture bounds the standard error a language server may write.
const stderrCapture = 1 << 20

// RootURI is the root of the function's crate, under which the client addresses
// its documents: file:///workspace/functions/<name>/.
func (s *LanguageSession) RootURI() string { return s.rootURI }

// Messages returns the language server's messages, one JSON-RPC message
// without framing each, with host paths replaced by the virtual workspace. The
// channel closes when the session has ended.
func (s *LanguageSession) Messages() <-chan []byte { return s.messages }

// Wait blocks until the session has ended and says why.
func (s *LanguageSession) Wait() LanguageEnd {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.end
}

// Close ends the session: the sandbox's process group is killed. It is safe
// to call more than once.
func (s *LanguageSession) Close() { s.cancel() }

// Send delivers one client message to the language server. The sequence
// number must be the one after the last accepted; the message passes the
// policy of [languagePolicy]: only methods the editor needs, only documents
// below the workspace and only the settings the builder chose. A message the
// policy refuses is answered to the client as a JSON-RPC error and is still
// consumed, so the sequence moves on.
func (s *LanguageSession) Send(seq uint64, msg []byte) error {
	if int64(len(msg)) > s.maxBytes {
		return fmt.Errorf("builder.LanguageSession.Send: %w: %d bytes, lsp.messageBytes = %d", ErrLanguageMessageSize, len(msg), s.maxBytes)
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if seq != s.nextSeq {
		return fmt.Errorf("builder.LanguageSession.Send: %w: sequence %d arrived; %d is next", ErrLanguageSequence, seq, s.nextSeq)
	}
	select {
	case <-s.done:
		return fmt.Errorf("builder.LanguageSession.Send: %w", ErrLanguageEnded)
	default:
	}
	s.nextSeq++
	forward, reply := s.policy.fromClient(msg)
	if reply != nil {
		s.emit(s.ctx, reply)
	}
	if forward == nil {
		return nil
	}
	return s.writeServer(forward)
}

// writeServer frames one message and writes it to the server's standard input.
func (s *LanguageSession) writeServer(body []byte) error {
	var buf bytes.Buffer
	buf.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	buf.Write(body)
	if _, err := s.stdin.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("builder.LanguageSession: write to the language server: %w", err)
	}
	return nil
}

// fromServer handles one message of the language server: requests the policy
// answers itself are answered, the rest is cleaned of host paths and queued.
func (s *LanguageSession) fromServer(ctx context.Context, raw []byte) error {
	forward, reply := s.policy.fromServer(raw)
	if reply != nil {
		// A failed write means the process is going; the session's end
		// says why, so the answer is dropped rather than reported.
		_ = s.writeServer(reply)
	}
	if forward != nil {
		s.emit(ctx, forward)
	}
	return nil
}

// emit queues a message for the consumer unless the session has ended.
func (s *LanguageSession) emit(ctx context.Context, msg []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.messages <- msg:
	case <-ctx.Done():
	}
}

// finish records how the process ended and closes the message channel.
func (s *LanguageSession) finish(ctx context.Context, res Result, runErr error, fw *frameReader, r *run) {
	end := LanguageEnd{Reason: "exited"}
	switch {
	case fw.bad.Load():
		end = LanguageEnd{Reason: "failed", Detail: "the language server sent a message that is malformed or over lsp.messageBytes"}
	case res.TimedOut:
		end = LanguageEnd{Reason: "time", Detail: "the session time passed"}
	case ctx.Err() != nil:
		end = LanguageEnd{Reason: "client"}
	case errors.Is(runErr, ErrSandboxUnavailable):
		end = LanguageEnd{Reason: "failed", Detail: "the kernel refuses the build sandbox"}
	case runErr != nil:
		end = LanguageEnd{Reason: "failed", Detail: "the language server could not run"}
	case res.OutputExceeded:
		end = LanguageEnd{Reason: "failed", Detail: "the language server wrote too much to its standard error"}
	case res.ExitCode != 0:
		detail := r.diagnostics(Result{Stderr: tail(res.Stderr, stderrTail)})
		end = LanguageEnd{Reason: "failed", Detail: fmt.Sprintf("the language server exited with status %d: %s", res.ExitCode, strings.TrimSpace(detail))}
	}
	s.mu.Lock()
	s.end, s.closed = end, true
	close(s.messages)
	s.mu.Unlock()
	close(s.done)
}

// tail returns at most n trailing bytes of b.
func tail(b []byte, n int) []byte {
	if len(b) > n {
		return b[len(b)-n:]
	}
	return b
}

// scrubWorkspace removes what could change the language server's settings from
// inside the user's sources: rust-analyzer reads `rust-analyzer.toml` files
// of the workspace, and cargo reads a `.cargo` directory of a crate. Neither
// may exist below the generated workspace root (FN-033). The root's own
// `.cargo/config.toml` is the builder's and stays.
func scrubWorkspace(workspace string) error {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return fmt.Errorf("builder.scrubWorkspace: %w", err)
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		switch {
		case !d.IsDir() && (name == "rust-analyzer.toml" || name == ".rust-analyzer.toml"):
			return root.Remove(p)
		case d.IsDir() && name == ".cargo" && p != ".cargo":
			if err := root.RemoveAll(p); err != nil {
				return fmt.Errorf("remove %s: %w", p, err)
			}
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("builder.scrubWorkspace: %w", err)
	}
	return nil
}

// frameReader parses the Content-Length framing of a language server's
// standard output and hands each body to deliver. It is written by one
// goroutine at a time.
type frameReader struct {
	limit   int64
	deliver func([]byte) error
	onBad   func()
	buf     []byte
	bad     atomic.Bool
}

// Write implements io.Writer. A malformed or oversize frame ends the session
// by calling onBad and refuses all further bytes.
func (f *frameReader) Write(p []byte) (int, error) {
	if f.bad.Load() {
		return 0, errors.New("builder: the language server's output is malformed")
	}
	f.buf = append(f.buf, p...)
	for {
		body, rest, ok, err := nextFrame(f.buf, f.limit)
		if err != nil {
			f.bad.Store(true)
			f.onBad()
			return 0, err
		}
		if !ok {
			return len(p), nil
		}
		f.buf = rest
		if err := f.deliver(body); err != nil {
			f.bad.Store(true)
			f.onBad()
			return 0, err
		}
	}
}

// nextFrame returns the first complete message of buf and the rest, or ok =
// false when more bytes are needed.
func nextFrame(buf []byte, limit int64) (body, rest []byte, ok bool, err error) {
	const headerEnd = "\r\n\r\n"
	end := bytes.Index(buf, []byte(headerEnd))
	if end < 0 {
		if len(buf) > 1024 {
			return nil, nil, false, errors.New("builder: a message header is too long")
		}
		return nil, nil, false, nil
	}
	length := int64(-1)
	for line := range strings.SplitSeq(string(buf[:end]), "\r\n") {
		name, value, found := strings.Cut(line, ":")
		if found && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			// 32 bits bound the length before the int conversion below.
			n, perr := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
			if perr != nil || n < 0 {
				return nil, nil, false, errors.New("builder: a message has no valid Content-Length")
			}
			length = n
		}
	}
	if length < 0 {
		return nil, nil, false, errors.New("builder: a message has no Content-Length")
	}
	if length > limit {
		return nil, nil, false, fmt.Errorf("builder: a message of %d bytes exceeds lsp.messageBytes = %d", length, limit)
	}
	start := end + len(headerEnd)
	if int64(len(buf)-start) < length {
		return nil, nil, false, nil
	}
	stop := start + int(length)
	return bytes.Clone(buf[start:stop]), buf[stop:], true, nil
}
