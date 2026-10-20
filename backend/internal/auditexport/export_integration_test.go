// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/ext/export/exporttest"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auditexport"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// ids generates UUIDv7 identifiers for the tests.
type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// clock is a clock the tests move.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// orgs visits one organisation.
type orgs struct{ id string }

func (o orgs) ForEachOrganization(_ context.Context, f func(string) error) error { return f(o.id) }

// notices records the reports of unreachable destinations.
type notices struct {
	mu   sync.Mutex
	list []auditexport.Unreachable
}

func (n *notices) DestinationUnreachable(_ context.Context, u auditexport.Unreachable) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.list = append(n.list, u)
	return nil
}

// fixture is an organisation with an audit chain, the service and the
// exporter.
type fixture struct {
	db      *storage.DB
	org     string
	log     *audit.Log
	svc     *auditexport.Service
	x       *auditexport.Exporter
	owner   auth.Principal
	clock   *clock
	notices *notices
	rx      *exporttest.Receiver
}

// newFixture builds a fixture whose batches hold four entries.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := storagetest.Open(t)
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	org, err := gen.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(context.Background(), `INSERT INTO organizations (id, key, name) VALUES ($1, 'acme', 'Acme')`, org); err != nil {
		t.Fatal(err)
	}
	file, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	set, err := limits.Defaults().Tighten(limits.AuditExportBatchEntries, limits.ScopeOrganization, 4)
	if err != nil {
		t.Fatal(err)
	}
	log := audit.NewLog(gen, nil)
	clk := &clock{t: time.Now().Add(time.Hour).Truncate(time.Microsecond)}
	o := auditexport.Options{
		DB: db, Audit: log, IDs: gen, Crypter: signing.CrypterOnly(file), Now: clk.now, Hostname: "plux-test",
		Limits: func(context.Context, pgx.Tx, string) (limits.Set, error) { return set, nil },
	}
	svc, err := auditexport.NewService(o)
	if err != nil {
		t.Fatal(err)
	}
	n := &notices{}
	x, err := auditexport.NewExporter(auditexport.ExporterOptions{
		Options: o, Organizations: orgs{id: org}, Notifier: n, Jitter: func() float64 { return 0 }, RunBudget: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{db: db, org: org, log: log, svc: svc, x: x, owner: auth.System(org), clock: clk, notices: n, rx: exporttest.NewReceiver(t)}
}

// appendEntries writes n entries to the organisation's chain.
func (f *fixture) appendEntries(t *testing.T, n int) {
	t.Helper()
	for range n {
		if err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
			_, err := f.log.Append(ctx, tx, audit.Entry{
				OrganizationID: f.org, Actor: audit.Actor{Kind: "user", ID: "u1", Display: "Ada"}, Action: audit.SecretSet,
				TargetKind: "environment", TargetID: "e1", Detail: `says "hi" ]`,
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// destination registers a syslog destination at the test receiver.
func (f *fixture) destination(t *testing.T) auditexport.Destination {
	t.Helper()
	d, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{
		Name: "siem", Kind: auditexport.KindSyslog, Address: f.rx.Address(), AllowPrivate: true, CAPEM: f.rx.CAPEM(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// status reads a destination and the chain head.
func (f *fixture) status(t *testing.T, id string) (auditexport.Destination, int64) {
	t.Helper()
	d, head, err := f.svc.Get(context.Background(), f.owner, id)
	if err != nil {
		t.Fatal(err)
	}
	return d, head
}

// run makes one pass of the exporter.
func (f *fixture) run(t *testing.T) {
	t.Helper()
	if err := f.x.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

var sequencePattern = regexp.MustCompile(`sequence="(\d+)"`)

// sequences lists the sequence numbers the receiver got, in arrival order.
func sequences(t *testing.T, rx *exporttest.Receiver) []int64 {
	t.Helper()
	var out []int64
	for _, m := range rx.Messages() {
		match := sequencePattern.FindStringSubmatch(m)
		if match == nil {
			t.Fatalf("a message has no sequence: %s", m)
		}
		n, _ := strconv.ParseInt(match[1], 10, 64)
		out = append(out, n)
	}
	return out
}

// assertComplete checks that sequences hold 1..head, each at least once and
// first seen in order.
func assertComplete(t *testing.T, got []int64, head int64) {
	t.Helper()
	seen := map[int64]bool{}
	var firsts []int64
	for _, s := range got {
		if !seen[s] {
			seen[s] = true
			firsts = append(firsts, s)
		}
	}
	for want := int64(1); want <= head; want++ {
		if !seen[want] {
			t.Errorf("entry %d never arrived (got %v)", want, got)
		}
	}
	if !slices.IsSorted(firsts) || int64(len(firsts)) != head {
		t.Errorf("entries first arrived as %v, want 1..%d in order", firsts, head)
	}
}

// Verifies: SEC-143.
// Every entry of the chain arrives once and in order, in batches no larger
// than audit.exportBatchEntries; the cursor and the status follow, and a
// later run sends only what is new.
func TestExporterDeliversTheChainInOrder_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 9)
	d := f.destination(t) // the registration is the tenth entry
	f.run(t)
	got, head := f.status(t, d.ID)
	if head != 10 || got.CursorSequence != 10 || got.ExportedCount != 10 || got.Failures != 0 || got.LastError != "" || got.LastSuccessAt.IsZero() {
		t.Errorf("status = %+v, head %d", got, head)
	}
	if want := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}; !slices.Equal(sequences(t, f.rx), want) {
		t.Errorf("sequences = %v, want %v", sequences(t, f.rx), want)
	}
	if n := f.rx.Connections(); n != 3 {
		t.Errorf("connections = %d, want 3 batches of at most 4", n)
	}
	f.run(t)
	if n := len(f.rx.Messages()); n != 10 {
		t.Errorf("a run with nothing new sent %d messages in all", n)
	}
	f.appendEntries(t, 2)
	f.run(t)
	if want := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}; !slices.Equal(sequences(t, f.rx), want) {
		t.Errorf("sequences = %v, want %v", sequences(t, f.rx), want)
	}
}

// wire is the part of a message's JSON a SIEM verifies the chain with.
type wire struct {
	ID           string    `json:"id"`
	Organization string    `json:"organization"`
	Sequence     int64     `json:"sequence"`
	Time         time.Time `json:"time"`
	Actor        struct{ Kind, ID, Display string }
	Action       string `json:"action"`
	Target       struct{ Kind, ID string }
	Detail       string `json:"detail"`
	PreviousHash string `json:"previousHash"`
	EntryHash    string `json:"entryHash"`
}

// Verifies: SEC-143, SEC-140.
// Each message carries the entry's hash and the previous hash, and a
// receiver that knows only the messages can recompute every hash and follow
// the chain.
func TestExportedRecordsLetASIEMVerifyTheChain_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 5)
	f.destination(t)
	f.run(t)
	previous := ""
	for i, m := range f.rx.Messages() {
		_, text, ok := strings.Cut(m, "\xEF\xBB\xBF")
		if !ok {
			t.Fatalf("message %d has no UTF-8 message: %s", i, m)
		}
		var w wire
		if err := json.Unmarshal([]byte(text), &w); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		e := audit.Entry{
			ID: w.ID, OrganizationID: w.Organization, Sequence: w.Sequence, At: w.Time,
			Actor:  audit.Actor{Kind: w.Actor.Kind, ID: w.Actor.ID, Display: w.Actor.Display},
			Action: audit.Action(w.Action), TargetKind: w.Target.Kind, TargetID: w.Target.ID, Detail: w.Detail,
			PreviousHash: w.PreviousHash,
		}
		if w.PreviousHash != previous {
			t.Errorf("message %d: previous hash %q, want %q", i, w.PreviousHash, previous)
		}
		if got := e.Hash(); got != w.EntryHash {
			t.Errorf("message %d: the hash recomputed from the message is %s, the message says %s", i, got, w.EntryHash)
		}
		if !strings.Contains(m, `entryHash="`+w.EntryHash+`"`) {
			t.Errorf("message %d: the structured data lacks the entry hash", i)
		}
		previous = w.EntryHash
	}
}

// Verifies: SEC-143.
// A receiver that resets connections mid-batch loses nothing: the failed
// batches are sent again after the backoff, the cursor never skips an entry,
// and every entry arrives at least once, first in order.
func TestExporterIsAtLeastOnceAcrossDisconnects_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 11)
	d := f.destination(t) // twelve entries in three batches
	f.rx.ResetAfterFrames(2, 3)
	for range 8 {
		f.run(t)
		f.clock.advance(20 * time.Minute)
		if got, head := f.status(t, d.ID); got.CursorSequence == head {
			break
		}
	}
	got, head := f.status(t, d.ID)
	if got.CursorSequence != head || head != 12 {
		t.Fatalf("cursor %d, head %d", got.CursorSequence, head)
	}
	assertComplete(t, sequences(t, f.rx), head)
	if len(f.rx.Messages()) <= 12 {
		t.Errorf("the reset batches were not sent again: %d messages", len(f.rx.Messages()))
	}
}

// Verifies: SEC-143.
// A destination that cannot be reached keeps its cursor, records PLX-8500
// and waits for an exponential backoff before the next attempt; nothing is
// tried before then, and a recovery clears the error.
func TestCursorDoesNotAdvanceOnFailure_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 5)
	d := f.destination(t)
	f.rx.RefuseHandshakes(true)
	f.run(t)
	got, _ := f.status(t, d.ID)
	if got.CursorSequence != 0 || got.CursorHash != "" || got.ExportedCount != 0 || got.Failures != 1 ||
		!strings.HasPrefix(got.LastError, "PLX-8500 ") || got.LastErrorAt.IsZero() {
		t.Fatalf("status after one failure = %+v", got)
	}
	if wait := got.NextAttemptAt.Sub(f.clock.now()); wait != 5*time.Second {
		t.Errorf("first wait = %s, want 5s (10s with the jitter at its lowest)", wait)
	}
	attempts := f.rx.Connections()
	f.run(t)
	if f.rx.Connections() != attempts {
		t.Error("a destination was tried again before its backoff passed")
	}
	f.clock.advance(6 * time.Second)
	f.run(t)
	got, _ = f.status(t, d.ID)
	if got.Failures != 2 || got.CursorSequence != 0 {
		t.Errorf("after the second failure: %+v", got)
	}
	if wait := got.NextAttemptAt.Sub(f.clock.now()); wait != 10*time.Second {
		t.Errorf("second wait = %s, want 10s", wait)
	}
	f.rx.RefuseHandshakes(false)
	f.clock.advance(time.Minute)
	f.run(t)
	got, head := f.status(t, d.ID)
	if got.CursorSequence != head || got.Failures != 0 || got.LastError != "" {
		t.Errorf("after recovery: %+v, head %d", got, head)
	}
	assertComplete(t, sequences(t, f.rx), head)
}

// Verifies: SEC-143.
// An unreachable destination is reported once, when it has failed three
// times in a row.
func TestUnreachableDestinationIsReported_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 1)
	d := f.destination(t)
	f.rx.RefuseHandshakes(true)
	for range 5 {
		f.run(t)
		f.clock.advance(time.Hour)
	}
	f.notices.mu.Lock()
	defer f.notices.mu.Unlock()
	if len(f.notices.list) != 1 {
		t.Fatalf("reports = %+v", f.notices.list)
	}
	if n := f.notices.list[0]; n.DestinationID != d.ID || n.OrganizationID != f.org || n.Failures != 3 || !strings.HasPrefix(n.LastError, "PLX-8500 ") {
		t.Errorf("report = %+v", n)
	}
}

// Verifies: SEC-143, SEC-104.
// A destination that was not allowed private addresses is refused when it
// names one, and when a public-looking name resolves to one the dial is
// refused and nothing is sent.
func TestPrivateDestinationsAreRefused_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{
		Name: "literal", Kind: auditexport.KindSyslog, Address: f.rx.Address(), CAPEM: f.rx.CAPEM(),
	})
	if code, _ := plxerr.CodeOf(err); code != plxerr.OutboundRequestBlocked {
		t.Errorf("a loopback address without the allowance: %v", err)
	}
	_, port, _ := strings.Cut(f.rx.Address(), ":")
	d, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{
		Name: "named", Kind: auditexport.KindSyslog, Address: "localhost:" + port, CAPEM: f.rx.CAPEM(),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.run(t)
	got, _ := f.status(t, d.ID)
	if got.CursorSequence != 0 || got.Failures != 1 || !strings.Contains(got.LastError, "not allowed") {
		t.Errorf("status = %+v", got)
	}
	if f.rx.Connections() != 0 || len(f.rx.Messages()) != 0 {
		t.Error("the receiver on a loopback address was reached")
	}
	if len(got.Warnings()) != 0 {
		t.Errorf("warnings = %v", got.Warnings())
	}
}

// Verifies: SEC-143.
// A destination allowed private addresses says so in its warnings, and its
// credentials are sealed: the status holds only a hint and the stored bytes
// hold neither the certificate nor the key.
func TestAllowedPrivateAddressWarnsAndCredentialsAreSealed_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	certPEM, keyPEM := clientCertificate(t)
	d, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{
		Name: "mtls", Kind: auditexport.KindSyslog, Address: f.rx.Address(), AllowPrivate: true, CAPEM: f.rx.CAPEM(),
		Credentials: auditexport.Credentials{ClientCertificatePEM: certPEM, ClientKeyPEM: keyPEM},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w := d.Warnings(); len(w) != 1 || w[0] != auditexport.WarningPrivateAddress {
		t.Errorf("warnings = %v", w)
	}
	if d.CredentialsHint != "client certificate" {
		t.Errorf("hint = %q", d.CredentialsHint)
	}
	var stored []byte
	if err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT credentials_ciphertext FROM audit_export_destinations WHERE id = $1`, d.ID).Scan(&stored)
	}); err != nil {
		t.Fatal(err)
	}
	if len(stored) == 0 || strings.Contains(string(stored), "PRIVATE KEY") || strings.Contains(string(stored), "CERTIFICATE") {
		t.Error("the credentials are not sealed")
	}
	f.run(t)
	got, head := f.status(t, d.ID)
	if got.CursorSequence != head {
		t.Errorf("a destination with a client certificate did not catch up: %+v", got)
	}
}

// Verifies: SEC-143, SEC-100.
// Registering, changing and removing a destination needs audit.export and is
// audited without the credentials; a mismatched certificate and key, a bad
// name and an enterprise kind without a licence are refused.
func TestServiceAuthorisationValidationAndAudit_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	nobody := auth.Principal{OrganizationID: f.org}
	if _, err := f.svc.Create(ctx, nobody, auditexport.NewDestination{Name: "siem", Kind: "syslog", Address: "siem.example.com:6514"}); err == nil {
		t.Error("a caller without audit.export registered a destination")
	}
	for name, n := range map[string]auditexport.NewDestination{
		"bad name":      {Name: "Siem!", Kind: "syslog", Address: "siem.example.com:6514"},
		"unpaired key":  {Name: "a", Kind: "syslog", Address: "siem.example.com:6514", Credentials: auditexport.Credentials{ClientKeyPEM: "x"}},
		"junk pair":     {Name: "a", Kind: "syslog", Address: "siem.example.com:6514", Credentials: auditexport.Credentials{ClientCertificatePEM: "x", ClientKeyPEM: "y"}},
		"junk roots":    {Name: "a", Kind: "syslog", Address: "siem.example.com:6514", CAPEM: "not a certificate"},
		"secret on tcp": {Name: "a", Kind: "syslog", Address: "siem.example.com:6514", Credentials: auditexport.Credentials{SigningSecret: "whsec_x"}},
		"cef":           {Name: "a", Kind: "cef", Address: "siem.example.com:6514"},
		"negative":      {Name: "a", Kind: "syslog", Address: "siem.example.com:6514", StartAfterSequence: -1},
		"no such entry": {Name: "a", Kind: "syslog", Address: "siem.example.com:6514", StartAfterSequence: 99},
	} {
		if _, err := f.svc.Create(ctx, f.owner, n); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	_, err := f.svc.Create(ctx, f.owner, auditexport.NewDestination{Name: "a", Kind: "cef", Address: "siem.example.com:6514"})
	if code, _ := plxerr.CodeOf(err); code != plxerr.EnterpriseFeatureNotLicensed {
		t.Errorf("cef without a licence: %v", err)
	}
	d, err := f.svc.Create(ctx, f.owner, auditexport.NewDestination{Name: "siem", Kind: "syslog", Address: "siem.example.com:6514"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, f.owner, auditexport.NewDestination{Name: "siem", Kind: "syslog", Address: "other.example.com:6514"}); err == nil {
		t.Error("a duplicate name was accepted")
	}
	updated, err := f.svc.Update(ctx, f.owner, auditexport.Change{ID: d.ID, Address: "other.example.com:6514", Enabled: false})
	if err != nil || updated.Address != "other.example.com:6514" || updated.Enabled || updated.Version != 2 {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if err := f.svc.Delete(ctx, f.owner, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Get(ctx, f.owner, d.ID); err == nil {
		t.Error("a deleted destination is still there")
	}
	var actions []audit.Action
	if err := f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		entries, err := f.log.List(ctx, tx, f.org, 0, 100)
		for _, e := range entries {
			actions = append(actions, e.Action)
			if strings.Contains(e.Detail, "example.com/") || strings.Contains(e.Detail, "whsec") {
				t.Errorf("the audit detail holds more than the host: %q", e.Detail)
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := []audit.Action{audit.ExportDestinationCreated, audit.ExportDestinationUpdated, audit.ExportDestinationDeleted}
	if !slices.Equal(actions, want) {
		t.Errorf("audited actions = %v, want %v", actions, want)
	}
}

// Verifies: SEC-143, LIM-001.
// An organisation cannot register more destinations than
// audit.exportDestinationsPerOrg allows.
func TestDestinationQuota_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for i := range int(limits.Defaults().Get(limits.AuditExportDestinationsPerOrg)) {
		if _, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{
			Name: "d" + strconv.Itoa(i), Kind: "syslog", Address: "siem.example.com:6514",
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{Name: "one-more", Kind: "syslog", Address: "siem.example.com:6514"})
	if code, _ := plxerr.CodeOf(err); code != plxerr.LimitExceeded {
		t.Errorf("err = %v, want PLX-%d", err, plxerr.LimitExceeded)
	}
}

// Verifies: SEC-143.
// A destination created after some entries can start after a sequence the
// receiver already has, and a disabled destination is not delivered to.
func TestStartAfterSequenceAndDisabled_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 6)
	d, err := f.svc.Create(context.Background(), f.owner, auditexport.NewDestination{
		Name: "late", Kind: "syslog", Address: f.rx.Address(), AllowPrivate: true, CAPEM: f.rx.CAPEM(), StartAfterSequence: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if want := []int64{5, 6, 7}; !slices.Equal(sequences(t, f.rx), want) {
		t.Errorf("sequences = %v, want %v", sequences(t, f.rx), want)
	}
	if _, err := f.svc.Update(context.Background(), f.owner, auditexport.Change{ID: d.ID, Address: f.rx.Address(), Enabled: false, AllowPrivate: true, CAPEM: f.rx.CAPEM()}); err != nil {
		t.Fatal(err)
	}
	f.appendEntries(t, 3)
	f.run(t)
	if n := len(f.rx.Messages()); n != 3 {
		t.Errorf("a disabled destination got %d messages in all", n)
	}
}

// Verifies: SEC-143, GOV-030.
// A destination of an enterprise kind whose licence has lapsed is not
// delivered to, keeps its cursor and says PLX-8600; no provider is called.
func TestEnterpriseKindWithoutLicenceDoesNotDeliver_SEC_143(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.appendEntries(t, 2)
	id, err := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_export_destinations (id, organization_id, name, kind, address) VALUES ($1, $2, 'cef', 'cef', 'siem.example.com:6514')`, id, f.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	got, _ := f.status(t, id)
	if got.CursorSequence != 0 || !strings.HasPrefix(got.LastError, "PLX-8600 ") {
		t.Errorf("status = %+v", got)
	}
}
