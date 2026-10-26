// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// languageSessions are the editor sessions of the role. The role holds them in
// memory only: a session lives as long as its stream, and nothing about it is
// stored (L-4).
type languageSessions struct {
	mu    sync.Mutex // guards byID and free
	byID  map[string]*builder.LanguageSession
	slots int
	free  int
}

func newLanguageSessions(n int) *languageSessions {
	return &languageSessions{byID: map[string]*builder.LanguageSession{}, slots: n, free: n}
}

// take reserves a slot, or reports that all are in use.
func (l *languageSessions) take() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.free == 0 {
		return false
	}
	l.free--
	return true
}

// add registers a started session under id.
func (l *languageSessions) add(id string, s *builder.LanguageSession) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byID[id] = s
}

// get returns the session of id.
func (l *languageSessions) get(id string) (*builder.LanguageSession, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.byID[id]
	return s, ok
}

// release forgets a session and returns its slot.
func (l *languageSessions) release(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byID, id)
	l.free++
}

// LanguageOpen implements fnpbconnect.BuilderServiceHandler: it starts the
// language server of one editor session in the build sandbox and streams its
// messages until the session ends (FN-033, ADR-0099). The session ends when
// the caller cancels the stream, the language server exits or the session time
// of lsp.sessionMinutes passes; the sandbox is killed then.
func (s *Service) LanguageOpen(ctx context.Context, req *connect.Request[fnpb.LanguageOpenRequest], stream *connect.ServerStream[fnpb.LanguageEvent]) error {
	if s.o.LanguageServer == "" || s.o.LanguageSessions < 1 {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("this builder runs no language server (builder.languageServer)"))
	}
	in := req.Msg
	if err := checkSource(in.GetSource(), in.GetSourceSha256(), in.GetName()); err != nil {
		return err
	}
	if !s.lang.take() {
		return connect.NewError(connect.CodeResourceExhausted, errors.New("the builder is running as many editor sessions as it may; retry later"))
	}
	id, err := s.sessionID()
	if err != nil {
		s.lang.release("")
		return err
	}
	defer s.lang.release(id)
	log := s.log.With(slog.String("session", id), slog.String("function", in.GetName()))
	if s.o.Unsandboxed {
		log.WarnContext(ctx, unsandboxedWarning)
	}
	sess, root, err := s.openLanguage(ctx, in)
	if err != nil {
		log.ErrorContext(ctx, "the language server did not start", slog.String("error", err.Error()))
		return err
	}
	defer sess.Close()
	s.lang.add(id, sess)
	log.InfoContext(ctx, "language session started")
	start := time.Now()
	if err := stream.Send(&fnpb.LanguageEvent{Event: &fnpb.LanguageEvent_Opened{Opened: &fnpb.LanguageOpened{SessionId: id, RootUri: root}}}); err != nil {
		return fmt.Errorf("builderrole.LanguageOpen: %w", err)
	}
	for msg := range sess.Messages() {
		if err := stream.Send(&fnpb.LanguageEvent{Event: &fnpb.LanguageEvent_Message{Message: msg}}); err != nil {
			sess.Close()
			end := sess.Wait()
			log.InfoContext(ctx, "language session stopped", slog.String("reason", end.Reason), slog.Duration("took", time.Since(start)))
			return fmt.Errorf("builderrole.LanguageOpen: %w", err)
		}
	}
	end := sess.Wait()
	log.InfoContext(ctx, "language session stopped", slog.String("reason", end.Reason), slog.Duration("took", time.Since(start)))
	if err := stream.Send(&fnpb.LanguageEvent{Event: &fnpb.LanguageEvent_Closed{Closed: &fnpb.LanguageClosed{Reason: end.Reason, Detail: end.Detail}}}); err != nil {
		return fmt.Errorf("builderrole.LanguageOpen: %w", err)
	}
	return nil
}

// openLanguage reads the source and starts the session within the registry's
// limits: the smaller of the requested and the registry maximum of
// lsp.sessionMinutes and function.buildMemory, and lsp.messageBytes.
func (s *Service) openLanguage(ctx context.Context, in *fnpb.LanguageOpenRequest) (*builder.LanguageSession, string, error) {
	files, err := builder.ExtractArchive(bytes.NewReader(in.GetSource()), maxSource())
	if err != nil {
		return nil, "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the source archive is not valid: %w", err))
	}
	decl, diags := builder.ReadDeclaration(s.loader, files, in.GetName(), builder.Placement(in.GetPlacement()))
	if decl == nil {
		return nil, "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the function document is not valid: %v", diags))
	}
	b, err := builder.New(s.toolConfig(nil))
	if err != nil {
		return nil, "", connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("the builder is not configured to run a language server: %w", err))
	}
	minutes := ceiling(limits.LspSessionMinutes, int64(in.GetSessionMinutes()))
	sess, err := b.OpenLanguage(ctx, decl.Source(files), builder.LanguageOptions{
		Server: s.o.LanguageServer, Args: s.o.LanguageArgs, Env: s.o.LanguageEnv,
		Wall:         time.Duration(minutes) * time.Minute,
		Memory:       uint64(ceiling(limits.FunctionBuildMemory, int64(min(in.GetMemoryBytes(), 1<<62)))), //nolint:gosec // G115: a ceiling of the registry is positive.
		MessageBytes: s.o.Limits.Get(limits.LspMessageBytes),
	})
	if err != nil {
		if pe, ok := errors.AsType[*plxerr.Error](err); ok {
			// The source's own fault, such as a crate the workspace cannot
			// be staged from; the editor shows it, nobody retries it.
			return nil, "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s: %s", pe.Code, pe.Message))
		}
		if errors.Is(err, builder.ErrSandboxUnavailable) {
			return nil, "", connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("the kernel refuses the build sandbox and builder.sandbox is not none: %w", err))
		}
		return nil, "", connect.NewError(connect.CodeInternal, err)
	}
	return sess, sess.RootURI(), nil
}

// sessionID returns a random identifier no one can guess: a client needs it to
// send to a session, and the caller binds it to the person who opened it.
func (s *Service) sessionID() (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(s.o.Random, raw[:]); err != nil {
		return "", connect.NewError(connect.CodeInternal, fmt.Errorf("session identifier: %w", err))
	}
	return hex.EncodeToString(raw[:]), nil
}

// LanguageSend implements fnpbconnect.BuilderServiceHandler: it delivers one
// message to the session's language server after the policy of the session
// has seen it (FN-033).
func (s *Service) LanguageSend(_ context.Context, req *connect.Request[fnpb.LanguageSendRequest]) (*connect.Response[fnpb.LanguageSendResponse], error) {
	in := req.Msg
	sess, ok := s.lang.get(in.GetSessionId())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such language-server session on this builder"))
	}
	switch err := sess.Send(in.GetSequence(), in.GetMessage()); {
	case err == nil:
	case errors.Is(err, builder.ErrLanguageSequence):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, builder.ErrLanguageMessageSize):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, builder.ErrLanguageEnded):
		return nil, connect.NewError(connect.CodeNotFound, err)
	default:
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&fnpb.LanguageSendResponse{}), nil
}

// Test implements fnpbconnect.BuilderServiceHandler: it builds the source in
// the sandbox and runs its stored test cases on the function host, and returns
// only the results (FN-034).
func (s *Service) Test(ctx context.Context, req *connect.Request[fnpb.TestRequest]) (*connect.Response[fnpb.TestResponse], error) {
	done, err := s.acquire()
	if err != nil {
		return nil, err
	}
	defer done()
	in := req.Msg.GetBuild()
	if s.o.Unsandboxed {
		s.log.WarnContext(ctx, unsandboxedWarning)
	}
	resp, err := s.build(ctx, in)
	if err != nil {
		s.log.ErrorContext(ctx, "the test run failed in the builder", slog.String("error", err.Error()))
		return nil, err
	}
	return connect.NewResponse(&fnpb.TestResponse{TestResults: resp.GetTestResults(), DiagnosticsJson: resp.GetDiagnosticsJson()}), nil
}

// checkSource refuses a request that is wrong rather than a source that does
// not build.
func checkSource(source []byte, wantSum, name string) error {
	if len(source) == 0 || name == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a session needs a source archive and a function name"))
	}
	sum := sha256.Sum256(source)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), wantSum) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the source archive does not match its SHA-256"))
	}
	return nil
}
