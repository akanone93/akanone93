// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// State is where a rollout is in its life.
type State string

// The states of a rollout. Scheduled, active and paused are open: the
// rollout holds its channel. Closing is open too: the worker is ending it.
const (
	StateScheduled  State = "scheduled"
	StateActive     State = "active"
	StatePaused     State = "paused"
	StateClosing    State = "closing"
	StateCompleted  State = "completed"
	StateEnded      State = "ended"
	StateAborted    State = "aborted"
	StateRolledBack State = "rolled_back"
)

// Open reports whether the state still holds its channel.
func (s State) Open() bool {
	return s == StateScheduled || s == StateActive || s == StatePaused || s == StateClosing
}

// Serving reports whether a rollout in the state offers its candidate to
// devices.
func (s State) Serving() bool { return s == StateActive || s == StatePaused }

// Rollout is a release offered to a share of a channel's devices.
type Rollout struct {
	ID            string
	AppID         string
	EnvironmentID string
	Environment   string
	ChannelID     string
	Channel       string
	// CandidateSequence is the release being rolled out, PreviousSequence the
	// channel's release when the rollout started (zero before).
	CandidateSequence, PreviousSequence int64
	State                               State
	// Reason says why the rollout is in its state, when it is not the course
	// of events: who paused it, or which gate failed.
	Reason      string
	Stages      []Stage
	StageIndex  int
	BasisPoints int
	StageBegan  time.Time
	StartAt     time.Time
	EndAt       time.Time
	Targeting   Targeting
	// AllowDevices always receive the candidate and DenyDevices never do.
	AllowDevices, DenyDevices []string
	Health                    Gate
	// LastCheck holds the verdict of the newest health check as JSON.
	LastCheck        json.RawMessage
	LastCheckAt      time.Time
	RevertedSequence int64
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time

	salt []byte
	// closeAction and closeState are set while the rollout is closing.
	closeAction, closeState string
}

// Includes reports whether a device is offered the candidate (REL-050,
// REL-051): never when denied, always when allowed, otherwise when it matches
// the targeting and its bucket lies inside the current share.
func (r Rollout) Includes(d Device) bool {
	if slices.Contains(r.DenyDevices, d.ID) {
		return false
	}
	if slices.Contains(r.AllowDevices, d.ID) {
		return true
	}
	return r.Targeting.Matches(d) && Admitted(Bucket(r.salt, Subject(d.UserHash, d.ID)), r.BasisPoints)
}

// rolloutOf converts a row.
func rolloutOf(row dbgen.Rollout) (Rollout, error) {
	r := Rollout{
		ID: storage.ID(row.ID), AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID), Environment: row.EnvironmentKey,
		ChannelID: storage.ID(row.ChannelID), Channel: row.ChannelKey, CandidateSequence: row.CandidateSequence,
		State: State(row.State), Reason: row.StateReason, StageIndex: int(row.StageIndex), BasisPoints: int(row.BasisPoints),
		StartAt: storage.Time(row.StartAt), LastCheck: row.LastCheck, CreatedBy: row.CreatedBy,
		CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt), salt: row.Salt,
		AllowDevices: ids(row.AllowDevices), DenyDevices: ids(row.DenyDevices),
	}
	if row.PreviousSequence != nil {
		r.PreviousSequence = *row.PreviousSequence
	}
	if row.RevertedSequence != nil {
		r.RevertedSequence = *row.RevertedSequence
	}
	if row.CloseAction != nil && row.CloseState != nil {
		r.closeAction, r.closeState = *row.CloseAction, *row.CloseState
	}
	if row.StageStartedAt.Valid {
		r.StageBegan = storage.Time(row.StageStartedAt)
	}
	if row.EndAt.Valid {
		r.EndAt = storage.Time(row.EndAt)
	}
	if row.LastCheckAt.Valid {
		r.LastCheckAt = storage.Time(row.LastCheckAt)
	}
	for _, part := range []struct {
		name string
		raw  []byte
		into any
	}{{"stages", row.Stages, &r.Stages}, {"targeting", row.Targeting, &r.Targeting}, {"health", row.Health, &r.Health}} {
		if err := json.Unmarshal(part.raw, part.into); err != nil {
			return Rollout{}, fmt.Errorf("rollout: the stored %s of %s: %w", part.name, r.ID, err)
		}
	}
	return r, nil
}

// ids renders UUIDs as strings.
func ids(in []pgtype.UUID) []string {
	out := make([]string, len(in))
	for i, id := range in {
		out[i] = storage.ID(id)
	}
	return out
}
