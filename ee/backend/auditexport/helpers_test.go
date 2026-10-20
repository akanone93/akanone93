// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/export"
)

// fixedLicence is an edition provider with a fixed status.
type fixedLicence struct{ status edition.Status }

// Status returns the fixed status.
func (f fixedLicence) Status() edition.Status { return f.status }

// licenceCtx returns a context whose licence is in state and lists the SIEM
// feature.
func licenceCtx(state edition.State) context.Context {
	return edition.WithProvider(context.Background(), fixedLicence{status: edition.Status{
		State:    state,
		Features: []edition.Feature{edition.FeatureSIEMExtended},
	}})
}

// licensedCtx returns a context with a valid licence for the SIEM feature.
func licensedCtx() context.Context { return licenceCtx(edition.StateValid) }

// sampleAt is the time of every sample record and the sinks' clock.
func sampleAt() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) }

// sampleRecords returns n records in chain order, each with a hash that
// names its sequence.
func sampleRecords(n int) []export.Record {
	records := make([]export.Record, 0, n)
	previous := ""
	for i := range n {
		seq := int64(i + 1)
		entry := "hash-" + strconv.FormatInt(seq, 10)
		records = append(records, export.Record{
			ID:             "rec-" + strconv.FormatInt(seq, 10),
			OrganizationID: "org-1",
			Sequence:       seq,
			At:             sampleAt().Add(time.Duration(i) * time.Second),
			ActorKind:      "user",
			ActorID:        "usr-1",
			ActorDisplay:   "Ada",
			Action:         "team.created",
			TargetKind:     "team",
			TargetID:       "t1",
			SourceIP:       "203.0.113.7",
			UserAgent:      "plux-test",
			EntryHash:      entry,
			PreviousHash:   previous,
		})
		previous = entry
	}
	return records
}

// neverDialled returns a dialler that fails the test if a sink opens a
// connection. A refused write must open none.
func neverDialled(t *testing.T) export.DialFunc {
	t.Helper()
	return func(context.Context, string, string) (net.Conn, error) {
		t.Error("the sink dialled without a licence")
		return nil, errors.New("dialled")
	}
}

// fixedClock is the sinks' clock.
func fixedClock() time.Time { return sampleAt() }

// testTLS is a configuration that verifies the receiver, as the core's
// configuration does.
func testTLS() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "siem.example.com"}
}
