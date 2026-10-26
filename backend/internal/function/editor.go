// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Editor is the route to the builder role's language-server sessions and test
// runs (FN-033, FN-034). Implementations are safe for concurrent use. The
// builder holds no database credential and no signing capability (L-4), so the
// service reads the source from the object store and sends it along.
type Editor interface {
	// OpenLanguage starts a session and calls send for each event until it
	// ends: the first is Opened, the last Closed. It returns when the
	// session is over or ctx ends.
	OpenLanguage(ctx context.Context, in EditorInput, send func(EditorEvent) error) error
	// SendLanguage delivers one client message to the session.
	SendLanguage(ctx context.Context, sessionID string, sequence uint64, message []byte) error
	// Test builds the source and runs its stored test cases.
	Test(ctx context.Context, in EditorInput) (EditorTests, error)
}

// EditorInput names the source of an editor session or test run and the limits
// it runs under.
type EditorInput struct {
	SourceObjectKey string
	SourceSHA256    string
	Name            string
	// SessionMinutes is lsp.sessionMinutes of the app (a language session).
	SessionMinutes uint32
	// MemoryBytes and Timeout are function.buildMemory and
	// function.buildTimeout of the app.
	MemoryBytes int64
	Timeout     time.Duration
}

// EditorEvent is one event of a language session: exactly one field is set.
type EditorEvent struct {
	Opened  *EditorOpened
	Message []byte
	Closed  *EditorClosed
}

// EditorOpened announces a started session.
type EditorOpened struct {
	SessionID string
	RootURI   string
	// Seconds is the time the session may still run.
	Seconds uint32
}

// EditorClosed says why a session ended.
type EditorClosed struct{ Reason, Detail string }

// EditorTests is the outcome of a test run.
type EditorTests struct {
	Results     []TestResult
	Diagnostics plxerr.Diagnostics
}

// EditorSession is a language session as the service tracks it.
type editorSession struct {
	organizationID string
	actor          audit.Actor
	appID          string
	cancel         context.CancelFunc
}

// editorRegistry tracks the live sessions of this process: who opened each, so
// that only that person may send to it, and how many an organisation runs.
type editorRegistry struct {
	mu     sync.Mutex // guards byID and perOrg
	byID   map[string]*editorSession
	perOrg map[string]int
}

// admit reserves a session of the organisation unless it already runs max.
func (r *editorRegistry) admit(org string, max int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perOrg == nil {
		r.perOrg = map[string]int{}
	}
	if int64(r.perOrg[org]) >= max {
		return false
	}
	r.perOrg[org]++
	return true
}

// bind names the session of a reservation.
func (r *editorRegistry) bind(id string, s *editorSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[string]*editorSession{}
	}
	r.byID[id] = s
}

// release returns a reservation and forgets the session, if it was named.
func (r *editorRegistry) release(org, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	if r.perOrg[org] > 0 {
		r.perOrg[org]--
	}
}

// find returns the session of id.
func (r *editorRegistry) find(id string) (*editorSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	return s, ok
}

// editorSource authorises the editor on a stored source and reads what the
// builder needs. Running a language server and the tests costs the
// organisation sandbox time, so it takes the permission that builds do
// (function.deploy on the app).
func (s *Service) editorSource(ctx context.Context, p auth.Principal, sourceID string) (EditorInput, string, limits.Set, error) {
	id, err := parseID(sourceID, "source")
	if err != nil {
		return EditorInput{}, "", limits.Set{}, err
	}
	var (
		in  EditorInput
		app string
		set limits.Set
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		src, err := dbgen.New(tx).GetFunctionSource(ctx, id)
		if err != nil {
			return failure(err, "function source")
		}
		app = storage.ID(src.AppID)
		if err := authorize(p, auth.FunctionDeploy, app); err != nil {
			return err
		}
		if set, err = s.o.Limits(ctx, tx, p.OrganizationID, app); err != nil {
			return fmt.Errorf("function: limits: %w", err)
		}
		in = EditorInput{
			SourceObjectKey: src.ObjectKey, SourceSHA256: hex.EncodeToString(src.Sha256), Name: src.Name,
			SessionMinutes: uint32(min(set.Get(limits.LspSessionMinutes), 1<<31)), //nolint:gosec // G115: bounded and non-negative.
			MemoryBytes:    set.Get(limits.FunctionBuildMemory),
			Timeout:        time.Duration(set.Get(limits.FunctionBuildTimeout)) * time.Millisecond,
		}
		return nil
	})
	if err == nil && s.o.Editor == nil {
		// After the permission check, so a caller who lacks it is refused
		// as such whether or not a builder is configured.
		err = plxerr.New(plxerr.FunctionBuildFailed, "this server has no builder to run the Rust editor")
	}
	return in, app, set, err
}

// OpenEditor starts a language-server session on an uploaded source and calls
// send for each event of the session until it ends (FN-033, ADR-0099). The
// caller needs function.deploy on the app. The organisation may run
// lsp.sessionsPerOrganization sessions at once on this server; the start and
// the end are audited with the reason. It returns when the session is over or
// ctx ends.
func (s *Service) OpenEditor(ctx context.Context, p auth.Principal, sourceID string, send func(EditorEvent) error) error {
	in, app, set, err := s.editorSource(ctx, p, sourceID)
	if err != nil {
		return err
	}
	if !s.editors.admit(p.OrganizationID, set.Get(limits.LspSessionsPerOrganization)) {
		return plxerr.New(plxerr.LimitExceeded, "the organisation already runs %d Rust editor sessions (lsp.sessionsPerOrganization); close one and retry",
			set.Get(limits.LspSessionsPerOrganization))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	session := &editorSession{organizationID: p.OrganizationID, actor: p.Actor(), appID: app, cancel: cancel}
	boundID := ""
	defer func() { s.editors.release(p.OrganizationID, boundID) }()

	reason := "client"
	started := false
	err = s.o.Editor.OpenLanguage(ctx, in, func(ev EditorEvent) error {
		switch {
		case ev.Opened != nil:
			boundID = ev.Opened.SessionID
			s.editors.bind(boundID, session)
			if err := s.recordEditor(ctx, p, audit.FunctionEditorStarted, boundID, fmt.Sprintf("function %s, source %s", in.Name, sourceID)); err != nil {
				return err
			}
			started = true
		case ev.Closed != nil:
			reason = ev.Closed.Reason
		}
		return send(ev)
	})
	if started {
		if err != nil && ctx.Err() == nil {
			reason = "failed"
		}
		note := fmt.Sprintf("function %s, reason %s", in.Name, reason)
		// The end is recorded even when the caller's context is gone.
		if recErr := s.recordEditor(context.WithoutCancel(ctx), p, audit.FunctionEditorStopped, boundID, note); recErr != nil {
			err = errors.Join(err, recErr)
		}
	}
	return err
}

// recordEditor appends one audit entry of an editor session.
func (s *Service) recordEditor(ctx context.Context, p auth.Principal, action audit.Action, session, detail string) error {
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		return s.record(ctx, tx, p, action, "function_editor", session, detail)
	})
}

// SendEditor delivers one client message to a session. Only the person who
// opened it may send, and only while they still hold function.deploy on the
// app; a person who lost it ends the session (SEC-101).
func (s *Service) SendEditor(ctx context.Context, p auth.Principal, sessionID string, sequence uint64, message []byte) error {
	if err := p.Authorize(auth.FunctionDeploy); err != nil {
		return fmt.Errorf("function: %w", err)
	}
	sess, ok := s.editors.find(sessionID)
	if !ok || sess.organizationID != p.OrganizationID || sess.actor != p.Actor() {
		return plxerr.New(plxerr.ResourceNotFound, "no such editor session")
	}
	if err := authorize(p, auth.FunctionDeploy, sess.appID); err != nil {
		sess.cancel()
		return err
	}
	if err := s.o.Editor.SendLanguage(ctx, sessionID, sequence, message); err != nil {
		return fmt.Errorf("function: editor: %w", err)
	}
	return nil
}

// RunEditorTests builds an uploaded source and runs its stored test cases in
// the build sandbox, returning one result per case (FN-034). Nothing is stored
// and nothing is deployed.
func (s *Service) RunEditorTests(ctx context.Context, p auth.Principal, sourceID string) (EditorTests, error) {
	in, _, _, err := s.editorSource(ctx, p, sourceID)
	if err != nil {
		return EditorTests{}, err
	}
	out, err := s.o.Editor.Test(ctx, in)
	if err != nil {
		return EditorTests{}, fmt.Errorf("function: editor tests: %w", err)
	}
	return out, nil
}
