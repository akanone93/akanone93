// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var _ document.DraftFollower = (*Service)(nil)

// DraftWritten compiles the stored draft of an app after an accepted write
// and pushes it to every live Studio session of the app, through the
// sessions' frame streams, so that a device on any replica receives it
// (L-14, contract #17, DEV-010). Studio never sends the edit itself. A
// session whose draft does not compile, or that cannot be pushed to, is
// left as it was and told why in its console; the write never fails for it.
func (s *Service) DraftWritten(_ context.Context, p auth.Principal, appID, pluginID string, paths []string) {
	if !s.CanPush() || len(paths) == 0 {
		return
	}
	s.drafter.submit(p, appID, pluginID, paths)
}

// draftWorkers reads the number of draft compiles that may run at once; 0
// when the registry cannot be read, so the next compile reads it again.
func (s *Service) draftWorkers(ctx context.Context, p auth.Principal, appID string) int {
	n := 0
	_ = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		set, err := s.o.Limits(ctx, tx, p.OrganizationID, appID)
		if err != nil {
			return err //nolint:wrapcheck // read again by the next compile
		}
		n = int(set.Get(limits.DevSessionDraftCompileWorkers))
		return nil
	})
	return max(n, 0)
}

// followApp compiles and pushes the paths written to an app to its sessions.
func (s *Service) followApp(ctx context.Context, p auth.Principal, appID string, paths []string) {
	app, err := storage.UUID(appID)
	if err != nil {
		return
	}
	var plans []pushPlan
	_ = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListFollowingDevSessions(ctx, dbgen.ListFollowingDevSessionsParams{AppID: app, Now: storage.Timestamp(s.now())})
		if err != nil {
			return err //nolint:wrapcheck // dropped: the write stands
		}
		for _, r := range rows {
			if plan, err := s.planOf(ctx, tx, p, sessionOf(r)); err == nil {
				plans = append(plans, plan)
			}
		}
		return nil
	})
	ids := slices.Clone(paths)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, plan := range plans {
		s.followDraft(ctx, p, plan, ids)
	}
}

// followDraft pushes the stored draft to one Studio session.
func (s *Service) followDraft(ctx context.Context, p auth.Principal, plan pushPlan, ids []string) {
	st := s.enter(plan)
	defer st.mu.Unlock()
	var files fstest.MapFS
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		files, err = s.reload(ctx, tx, st, plan)
		return err
	})
	if err != nil {
		s.reportDraft(plan, PushResult{}, fmt.Errorf("devsession: read the draft: %w", err))
		return
	}
	out, err := s.compileAndPush(ctx, p, plan, st, files, st.revs, ids, nil)
	// The write stands either way and the next one pushes again; a draft that did not reach the session's devices
	// says why, in the server's log and in the session's console, instead of leaving a phone waiting in silence.
	s.reportDraft(plan, out, err)
}

// maxReportedDiagnostics bounds the errors of a refused draft put in the session's console.
const maxReportedDiagnostics = 20

// reportDraft tells the session, as error events in its console, and the server's log why its draft was not pushed:
// the compiler's errors, or the failure to compile, sign or deliver it. A pushed draft reports nothing.
func (s *Service) reportDraft(plan pushPlan, out PushResult, err error) {
	var errs []*pluxv1.DevError
	switch {
	case err != nil:
		errs = append(errs, devErrorOf(err))
	case out.Diagnostics.HasErrors():
		for _, d := range out.Diagnostics {
			if d.Severity < plxerr.SeverityError {
				continue
			}
			if len(errs) == maxReportedDiagnostics {
				break
			}
			msg := d.Message
			if d.File != "" {
				msg = d.File + " " + d.Path + ": " + msg
			}
			errs = append(errs, &pluxv1.DevError{Code: d.Code.String(), Reason: string(d.Reason), Message: msg})
		}
	default:
		return
	}
	// The log names the codes only: a message may quote the draft, which the session's own console may show.
	codes := make([]string, 0, len(errs))
	for _, e := range errs {
		codes = append(codes, e.GetCode())
	}
	s.o.Log.Warn("devsession: the draft was not pushed to the session's devices",
		"session", plan.session.ID, "app", plan.session.AppID, "environment", plan.session.EnvironmentKey,
		"codes", codes)
	room := s.hub.room(plan.session.ID, nil)
	for _, e := range errs {
		ev := &pluxv1.DevEvent{
			TimeUnixMicros: s.now().UnixMicro(),
			Payload: &pluxv1.DevEvent_Error{Error: &pluxv1.ErrorEvent{
				Error: e, Severity: pluxv1.Severity_SEVERITY_ERROR,
			}},
		}
		_ = room.appendEvent("", 0, ev) //nolint:contextcheck,errcheck // best effort: the console may have gone
	}
}

// devErrorOf is an error as the console shows it: its PLX code where it has one.
func devErrorOf(err error) *pluxv1.DevError {
	var pe *plxerr.Error
	if errors.As(err, &pe) {
		return &pluxv1.DevError{Code: pe.Code.String(), Message: pe.Message}
	}
	return &pluxv1.DevError{Code: plxerr.DevPatchRejected.String(), Message: err.Error()}
}

// welcomeTimeout bounds the compile and push of a welcome.
const welcomeTimeout = 2 * time.Minute

// welcome pushes the stored draft, whole, to a device of a Studio session
// that connected without having carried out any frame: a fresh pairing, a
// re-pairing or a restarted app. Such a device shows the draft at once,
// not whatever release it held, and holds every bundle the next pushes
// leave out. It runs as the server's own work (auth.System, audited as
// such) and sends the device nothing it may not see: the session's draft,
// which the next edit would push to it anyway. A session that follows no
// draft (a CLI session, which pushes its own files) is left alone, and a
// draft that does not compile is not pushed.
func (s *Service) welcome(b DeviceBinding) {
	if !s.CanPush() {
		return
	}
	s.forgetDevice(b.SessionID, b.DeviceID)
	ctx, cancel := context.WithTimeout(s.hub.ctx, welcomeTimeout)
	defer cancel()
	p := auth.System(b.OrganizationID)
	var (
		plan pushPlan
		ok   bool
	)
	_ = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetDevSession(ctx, storage.MustUUID(b.SessionID))
		if err != nil {
			return err //nolint:wrapcheck // dropped: the device keeps what it shows
		}
		session := sessionOf(row)
		if session.Kind != KindStudio {
			return nil
		}
		plan, err = s.planOf(ctx, tx, p, session)
		ok = err == nil
		return err
	})
	if ok {
		s.welcomeDraft(ctx, p, plan, b.DeviceID)
	}
}

// welcomeDraft sends a device that connected afresh the stored draft. While
// the draft is what this replica last signed, the device is sent that signed
// bundle again, whole: a device that reconnects in a loop, by fault or on
// purpose, costs a read of the draft, not a compile, a signature, an audit
// entry and a push to every device of the session each time (#275). A draft
// that changed since is compiled and pushed as an edit is.
func (s *Service) welcomeDraft(ctx context.Context, p auth.Principal, plan pushPlan, deviceID string) {
	st := s.enter(plan)
	defer st.mu.Unlock()
	var files fstest.MapFS
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		files, err = s.reload(ctx, tx, st, plan)
		return err
	})
	if err != nil {
		s.reportDraft(plan, PushResult{}, fmt.Errorf("devsession: read the draft: %w", err))
		return
	}
	if last := st.last; last != nil && sameFiles(last.files, files) {
		held := map[string]bool{}
		st.sent[deviceID] = held
		whole := func(string) *pluxv1.DevBundleFrame {
			f := proto.CloneOf(last.frame)
			f.Sections = make(map[string][]byte, len(last.bundles))
			for ref, data := range last.bundles {
				f.Sections[ref] = data
				held[ref] = true
			}
			return f
		}
		s.deliver(plan.session.ID, []Device{{ID: deviceID}}, whole, nil, nil)
		return
	}
	out, err := s.compileAndPush(ctx, p, plan, st, files, st.revs, nil, nil)
	s.reportDraft(plan, out, err)
}
