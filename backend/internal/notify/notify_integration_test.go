// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// world is an organisation with an owner, a developer and a viewer, a team
// holding the developer and the viewer, and a notify service over a fresh
// schema whose clock the test moves.
type world struct {
	db        *storage.DB
	svc       *notify.Service
	log       *audit.Log
	org       string
	owner     auth.Principal
	developer auth.Principal
	viewer    auth.Principal
	team      string
	clock     atomic.Int64
	limits    atomic.Pointer[limits.Set]
}

func (w *world) now() time.Time { return time.Unix(w.clock.Load(), 0).UTC() }

func (w *world) advance(d time.Duration) { w.clock.Add(int64(d / time.Second)) }

func newWorld(t *testing.T, mutate func(*notify.Options)) *world {
	t.Helper()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	a, err := auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: a, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"})
	if err != nil {
		t.Fatal(err)
	}
	w := &world{db: db, log: log}
	w.clock.Store(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).Unix())
	set := limits.Defaults()
	w.limits.Store(&set)
	o := notify.Options{
		DB: db, Audit: log, Crypter: backend, IDs: gen,
		Policy:        httpx.Policy{AllowLoopback: true},
		AllowInsecure: true,
		Now:           w.now,
		Jitter:        func() float64 { return 0 },
		Limits:        func(context.Context, string) (limits.Set, error) { return *w.limits.Load(), nil },
	}
	o.HTTP = httpx.NewClient(httpx.Options{Policy: o.Policy, Timeout: 5 * time.Second})
	if mutate != nil {
		mutate(&o)
	}
	if w.svc, err = notify.New(o); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	admin, invitation, err := a.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	adminID := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}
	org, err := ten.CreateOrganization(ctx, adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	w.org = org.ID
	if w.owner, err = a.Resolve(ctx, adminID, w.org); err != nil {
		t.Fatal(err)
	}
	team, err := ten.CreateTeam(ctx, w.owner, "ops", "Operations")
	if err != nil {
		t.Fatal(err)
	}
	w.team = team.ID
	member := func(email, role, teamID string) auth.Principal {
		m, _, err := ten.AddMember(ctx, w.owner, "", email, role)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := ten.AddMember(ctx, w.owner, teamID, email, ""); err != nil {
			t.Fatal(err)
		}
		p, err := a.Resolve(ctx, auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: role, SecondFactor: true}, w.org)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	w.developer = member("dev@example.com", "developer", w.team)
	w.viewer = member("viewer@example.com", "viewer", w.team)
	return w
}

func (w *world) event(t notify.EventType, title string) notify.Event {
	return notify.Event{Type: t, OrganizationID: w.org, Title: title, Body: "details of " + title, Link: "/apps/demo/releases/7", Attributes: map[string]string{"release": "7"}}
}

// ---- types and validation ------------------------------------------------

// SRV-062: the event types are a closed, sorted registry.
func TestSRV062EventTypeRegistry(t *testing.T) {
	want := []notify.EventType{
		notify.AlertFired, notify.ApprovalRequested, notify.BreakGlassUsed, notify.FunctionFailing,
		notify.LicenceExpiring, notify.LockRequested, notify.ReleasePublished, notify.RolloutPaused,
	}
	got := notify.Types()
	if len(got) != len(want) {
		t.Fatalf("%d event types, want %d", len(got), len(want))
	}
	for i, ti := range got {
		if ti.Type != want[i] || ti.Description == "" || ti.Severity == "" || !notify.Known(ti.Type) {
			t.Errorf("type %d = %+v, want %s", i, ti, want[i])
		}
	}
	if notify.Known("made.up") {
		t.Error("an unregistered type is known")
	}
}

func TestSRV062PublishValidation(t *testing.T) {
	w := newWorld(t, nil)
	ctx := context.Background()
	bad := []notify.Event{
		{Type: "made.up", OrganizationID: w.org, Title: "x"},
		{Type: notify.AlertFired, Title: "x"},
		{Type: notify.AlertFired, OrganizationID: w.org},
		{Type: notify.AlertFired, OrganizationID: w.org, Title: strings.Repeat("x", 201)},
		{Type: notify.AlertFired, OrganizationID: w.org, Title: "x", Link: "javascript:alert(1)"},
		{Type: notify.AlertFired, OrganizationID: w.org, Title: "x", Attributes: map[string]string{"": "v"}},
	}
	for i, e := range bad {
		if err := w.svc.Publish(ctx, e); err == nil {
			t.Errorf("event %d was accepted", i)
		}
	}
}

func TestSRV063Backoff(t *testing.T) {
	prev := time.Duration(0)
	for failed := 1; failed <= 12; failed++ {
		lo, hi := notify.Backoff(failed, 0), notify.Backoff(failed, 0.9999)
		if lo <= 0 || hi < lo || hi > 4*time.Hour {
			t.Errorf("failed=%d: [%s, %s]", failed, lo, hi)
		}
		if lo < prev {
			t.Errorf("failed=%d: %s is below the previous step %s", failed, lo, prev)
		}
		prev = lo
	}
	if got := notify.Backoff(1, 0); got != 15*time.Second {
		t.Errorf("first retry = %s, want 15s with no jitter", got)
	}
	if got := notify.Backoff(50, 0.9999); got > 4*time.Hour {
		t.Errorf("capped step = %s", got)
	}
	// 24 hours hold a useful number of attempts.
	var sum time.Duration
	attempts := 0
	for sum < notify.DeliveryWindow {
		attempts++
		sum += notify.Backoff(attempts, 1)
	}
	if attempts < 10 || attempts > 40 {
		t.Errorf("%d attempts fit in 24h", attempts)
	}
}

// ---- inbox and subscriptions ---------------------------------------------

// SRV-062: a user and a team follow event types into the inbox, once each.
func TestSRV062InboxAndSubscriptions(t *testing.T) {
	w := newWorld(t, nil)
	ctx := context.Background()

	// A person follows an event for themselves.
	if _, err := w.svc.Subscribe(ctx, w.developer, notify.Subscription{EventType: notify.ReleasePublished}); err != nil {
		t.Fatalf("self-subscribe: %v", err)
	}
	// Twice is not an error and not a second entry.
	if _, err := w.svc.Subscribe(ctx, w.developer, notify.Subscription{EventType: notify.ReleasePublished}); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	// A team needs notifications.manage.
	if _, err := w.svc.Subscribe(ctx, w.developer, notify.Subscription{SubjectKind: "team", SubjectID: w.team, EventType: notify.AlertFired}); code(err) != plxerr.PermissionDenied {
		t.Fatalf("a developer subscribed a team: %v", err)
	}
	if _, err := w.svc.Subscribe(ctx, w.owner, notify.Subscription{SubjectKind: "team", SubjectID: w.team, EventType: notify.ReleasePublished}); err != nil {
		t.Fatalf("team subscribe: %v", err)
	}
	if _, err := w.svc.Subscribe(ctx, w.owner, notify.Subscription{SubjectKind: "team", SubjectID: w.team, EventType: "made.up"}); code(err) != plxerr.InvalidEnumValue {
		t.Fatalf("unknown event type: %v", err)
	}
	if _, err := w.svc.Subscribe(ctx, w.owner, notify.Subscription{SubjectKind: "team", SubjectID: "00000000-0000-7000-8000-000000000000", EventType: notify.AlertFired}); code(err) != plxerr.ResourceNotFound {
		t.Fatalf("unknown team: %v", err)
	}

	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "Release 7")); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.Publish(ctx, w.event(notify.RolloutPaused, "Paused")); err != nil {
		t.Fatal(err)
	}

	// The developer follows twice (self and team) and gets one entry; the
	// viewer follows through the team; the owner follows nothing.
	items, unread, err := w.svc.Inbox(ctx, w.developer, false, 10)
	if err != nil || len(items) != 1 || unread != 1 {
		t.Fatalf("developer inbox = %+v unread %d, %v", items, unread, err)
	}
	if it := items[0]; it.Title != "Release 7" || it.Type != notify.ReleasePublished || it.Link != "/apps/demo/releases/7" || it.Read {
		t.Fatalf("item = %+v", it)
	}
	if items, _, err := w.svc.Inbox(ctx, w.viewer, false, 10); err != nil || len(items) != 1 {
		t.Fatalf("viewer inbox = %+v, %v", items, err)
	}
	if items, _, err := w.svc.Inbox(ctx, w.owner, false, 10); err != nil || len(items) != 0 {
		t.Fatalf("owner inbox = %+v, %v", items, err)
	}

	// Read and unread.
	n, err := w.svc.MarkRead(ctx, w.developer, []string{items0(t, w, w.developer)})
	if err != nil || n != 1 {
		t.Fatalf("MarkRead = %d, %v", n, err)
	}
	if items, unread, _ := w.svc.Inbox(ctx, w.developer, true, 10); len(items) != 0 || unread != 0 {
		t.Fatalf("unread inbox = %+v %d", items, unread)
	}
	if items, _, _ := w.svc.Inbox(ctx, w.developer, false, 10); len(items) != 1 || !items[0].Read {
		t.Fatalf("read inbox = %+v", items)
	}
	// Another person's notification cannot be marked.
	if n, err := w.svc.MarkRead(ctx, w.developer, []string{items0(t, w, w.viewer)}); err != nil || n != 0 {
		t.Fatalf("marking the viewer's notification: %d, %v", n, err)
	}

	// Own subscriptions list; everyone's need the permission.
	own, err := w.svc.ListSubscriptions(ctx, w.developer, false)
	if err != nil || len(own) != 1 {
		t.Fatalf("own subscriptions = %+v, %v", own, err)
	}
	if _, err := w.svc.ListSubscriptions(ctx, w.developer, true); code(err) != plxerr.PermissionDenied {
		t.Fatalf("all subscriptions: %v", err)
	}
	if all, err := w.svc.ListSubscriptions(ctx, w.owner, true); err != nil || len(all) != 2 {
		t.Fatalf("all subscriptions = %+v, %v", all, err)
	}
	if err := w.svc.Unsubscribe(ctx, w.viewer, own[0].ID); code(err) != plxerr.PermissionDenied {
		t.Fatalf("unsubscribing someone else: %v", err)
	}
	if err := w.svc.Unsubscribe(ctx, w.developer, own[0].ID); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
}

func items0(t *testing.T, w *world, p auth.Principal) string {
	t.Helper()
	items, _, err := w.svc.Inbox(context.Background(), p, false, 10)
	if err != nil || len(items) == 0 {
		t.Fatalf("inbox: %+v %v", items, err)
	}
	return items[0].ID
}

// SRV-062: notifications.perHour drops the excess and audits it once.
func TestSRV062PerHourLimit(t *testing.T) {
	w := newWorld(t, nil)
	ctx := context.Background()
	set, err := limits.Defaults().Tighten(limits.NotificationsPerHour, limits.ScopeOrganization, 3)
	if err != nil {
		t.Fatal(err)
	}
	w.limits.Store(&set)
	if _, err := w.svc.Subscribe(ctx, w.owner, notify.Subscription{SubjectKind: "team", SubjectID: w.team, EventType: notify.AlertFired}); err != nil {
		t.Fatal(err)
	}
	// Two team members: each event makes two inbox entries.
	for i := range 4 {
		if err := w.svc.Publish(ctx, w.event(notify.AlertFired, "alert "+string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for _, p := range []auth.Principal{w.developer, w.viewer} {
		items, _, _ := w.svc.Inbox(ctx, p, false, 50)
		total += len(items)
	}
	if total != 3 {
		t.Fatalf("%d notifications created, want the limit of 3", total)
	}
	if n := countAudit(t, w, audit.NotificationsDropped); n != 1 {
		t.Fatalf("%d drop entries, want one for the hour", n)
	}
	// The next hour starts fresh.
	w.advance(time.Hour)
	if err := w.svc.Publish(ctx, w.event(notify.AlertFired, "again")); err != nil {
		t.Fatal(err)
	}
	if items, _, _ := w.svc.Inbox(ctx, w.developer, false, 50); items[0].Title != "again" {
		t.Fatalf("the new hour's first notification is %q", items[0].Title)
	}
}

func countAudit(t *testing.T, w *world, action audit.Action) int {
	t.Helper()
	var n int
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1`, string(action)).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// ---- channels ------------------------------------------------------------

type request struct {
	path string
	body string
}

// capture serves a webhook that records what it receives and answers with status.
type capture struct {
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []request
	header []http.Header
	status atomic.Int32
}

func newCapture(t *testing.T) *capture {
	t.Helper()
	c := &capture{}
	c.status.Store(200)
	c.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, request{path: r.URL.Path, body: string(b)})
		c.header = append(c.header, r.Header.Clone())
		c.mu.Unlock()
		rw.WriteHeader(int(c.status.Load()))
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *capture) last(t *testing.T) (request, http.Header) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		t.Fatal("the destination received nothing")
	}
	return c.reqs[len(c.reqs)-1], c.header[len(c.header)-1]
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

// SRV-062: Slack, Teams and Telegram receive formatted messages, the secret
// URL and token never reach the delivery log, and a failure is retried.
func TestSRV062ChatChannels(t *testing.T) {
	cap := newCapture(t)
	w := newWorld(t, func(o *notify.Options) { o.TelegramBase = cap.srv.URL; o.StudioURL = "https://studio.example" })
	ctx := context.Background()
	const token = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdef"
	specs := []notify.ChannelSpec{
		{Name: "slack", Kind: notify.KindSlack, Secret: cap.srv.URL + "/services/T000/B000/SECRETSLACKKEY"},
		{Name: "teams", Kind: notify.KindTeams, Secret: cap.srv.URL + "/workflows/SECRETTEAMSKEY"},
		{Name: "telegram", Kind: notify.KindTelegram, Settings: map[string]string{"chat_id": "-100123"}, Secret: token},
	}
	if _, err := w.svc.CreateChannel(ctx, w.developer, specs[0]); code(err) != plxerr.PermissionDenied {
		t.Fatalf("a developer created a channel: %v", err)
	}
	for _, spec := range specs {
		ch, err := w.svc.CreateChannel(ctx, w.owner, spec)
		if err != nil {
			t.Fatalf("%s: %v", spec.Name, err)
		}
		if strings.Contains(ch.Hint, "SECRETSLACK") || len(ch.Hint) > 8 {
			t.Errorf("hint %q gives away too much", ch.Hint)
		}
		if _, err := w.svc.Subscribe(ctx, w.owner, notify.Subscription{SubjectKind: "team", SubjectID: w.team, EventType: notify.ApprovalRequested, ChannelID: ch.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.svc.Publish(ctx, w.event(notify.ApprovalRequested, "Approve <release> 7 & co")); err != nil {
		t.Fatal(err)
	}
	if n, err := w.svc.DeliverDue(ctx, w.org); err != nil || n != 3 {
		t.Fatalf("DeliverDue = %d, %v; want one message per chat channel, not per member", n, err)
	}
	cap.mu.Lock()
	bodies := map[string]string{}
	for _, r := range cap.reqs {
		bodies[r.path] = r.body
	}
	cap.mu.Unlock()
	slack := bodies["/services/T000/B000/SECRETSLACKKEY"]
	if !strings.Contains(slack, `*Approve \u0026lt;release\u0026gt; 7 \u0026amp; co*`) || !strings.Contains(slack, "https://studio.example/apps/demo/releases/7") {
		t.Errorf("slack body = %s", slack)
	}
	teams := bodies["/workflows/SECRETTEAMSKEY"]
	if !strings.Contains(teams, "AdaptiveCard") || !strings.Contains(teams, "Action.OpenUrl") || !strings.Contains(teams, "Approve \\u003crelease\\u003e 7") {
		t.Errorf("teams body = %s", teams)
	}
	tg := bodies["/bot"+token+"/sendMessage"]
	if !strings.Contains(tg, `"chat_id":"-100123"`) || !strings.Contains(tg, `"parse_mode":"HTML"`) || !strings.Contains(tg, "\\u003cb\\u003eApprove \\u0026lt;release") {
		t.Errorf("telegram body = %s", tg)
	}

	// The log shows delivered, with no destination in it.
	log, err := w.svc.Deliveries(ctx, w.owner, notify.DeliveryFilter{Kind: "channel"})
	if err != nil || len(log) != 3 {
		t.Fatalf("delivery log = %+v, %v", log, err)
	}
	for _, d := range log {
		if d.State != "delivered" || d.Attempts != 1 || d.LastStatus != 200 {
			t.Errorf("delivery = %+v", d)
		}
	}
	if _, err := w.svc.Deliveries(ctx, w.developer, notify.DeliveryFilter{Kind: "channel"}); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer read the delivery log: %v", err)
	}

	// A failing destination is retried later and the error text is clean.
	cap.status.Store(500)
	if err := w.svc.Publish(ctx, w.event(notify.ApprovalRequested, "Second")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.DeliverDue(ctx, w.org); err != nil {
		t.Fatal(err)
	}
	log, _ = w.svc.Deliveries(ctx, w.owner, notify.DeliveryFilter{Kind: "channel", State: "pending"})
	if len(log) != 3 {
		t.Fatalf("%d pending deliveries, want 3", len(log))
	}
	for _, d := range log {
		if d.Attempts != 1 || d.LastStatus != 500 || !d.NextAttempt.After(w.now()) {
			t.Errorf("pending delivery = %+v", d)
		}
		if strings.Contains(d.LastError, "SECRET") || strings.Contains(d.LastError, token) {
			t.Errorf("the delivery log leaks a secret: %q", d.LastError)
		}
	}
	// Not due yet: nothing is attempted before the backoff has passed.
	if n, _ := w.svc.DeliverDue(ctx, w.org); n != 0 {
		t.Fatalf("%d attempts before the backoff", n)
	}
	cap.status.Store(200)
	w.advance(time.Minute)
	if n, err := w.svc.DeliverDue(ctx, w.org); err != nil || n != 3 {
		t.Fatalf("retry = %d, %v", n, err)
	}
	if log, _ := w.svc.Deliveries(ctx, w.owner, notify.DeliveryFilter{Kind: "channel", State: "pending"}); len(log) != 0 {
		t.Fatalf("%d still pending", len(log))
	}
}

// SRV-062, SEC-105: a channel naming an internal address is refused, and so
// is a misconfiguration (PLX-8400).
func TestSRV062ChannelMisconfigured(t *testing.T) {
	w := newWorld(t, func(o *notify.Options) { o.Policy = httpx.Policy{}; o.AllowInsecure = false })
	ctx := context.Background()
	for name, spec := range map[string]notify.ChannelSpec{
		"loopback":       {Name: "a", Kind: notify.KindSlack, Secret: "https://127.0.0.1/hook"},
		"metadata":       {Name: "b", Kind: notify.KindTeams, Secret: "https://169.254.169.254/latest"},
		"private":        {Name: "c", Kind: notify.KindSlack, Secret: "https://10.0.0.5/hook"},
		"http":           {Name: "d", Kind: notify.KindSlack, Secret: "http://hooks.example.com/x"},
		"credentials":    {Name: "e", Kind: notify.KindSlack, Secret: "https://user:pw@hooks.example.com/x"},
		"no url":         {Name: "f", Kind: notify.KindSlack},
		"bad token":      {Name: "g", Kind: notify.KindTelegram, Settings: map[string]string{"chat_id": "1"}, Secret: "nope"},
		"bad chat":       {Name: "h", Kind: notify.KindTelegram, Settings: map[string]string{"chat_id": "a b"}, Secret: "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdef"},
		"unknown kind":   {Name: "i", Kind: "pager", Secret: "x"},
		"unknown field":  {Name: "j", Kind: notify.KindSlack, Settings: map[string]string{"color": "red"}, Secret: "https://hooks.example.com/x"},
		"smtp no host":   {Name: "k", Kind: notify.KindSMTP, Settings: map[string]string{"from": "a@example.com"}},
		"smtp bad from":  {Name: "l", Kind: notify.KindSMTP, Settings: map[string]string{"host": "mail.example.com", "from": "not an address"}},
		"smtp injection": {Name: "m", Kind: notify.KindSMTP, Settings: map[string]string{"host": "mail.example.com", "from": "a@example.com\r\nBcc: x@example.com"}},
		"smtp no tls":    {Name: "n", Kind: notify.KindSMTP, Settings: map[string]string{"host": "mail.example.com", "from": "a@example.com", "tls": "none"}},
		"smtp user":      {Name: "o", Kind: notify.KindSMTP, Settings: map[string]string{"host": "mail.example.com", "from": "a@example.com", "username": "u"}},
		"empty name":     {Name: "", Kind: notify.KindSlack, Secret: "https://hooks.example.com/x"},
	} {
		_, err := w.svc.CreateChannel(ctx, w.owner, spec)
		if name == "unknown kind" || name == "empty name" {
			if err == nil {
				t.Errorf("%s: accepted", name)
			}
			continue
		}
		if code(err) != plxerr.ChannelMisconfigured {
			t.Errorf("%s: %v, want PLX-8400", name, err)
		}
	}
	if _, err := w.svc.CreateChannel(ctx, w.owner, notify.ChannelSpec{Name: "ok", Kind: notify.KindSlack, Secret: "https://hooks.slack.com/services/T/B/x"}); err != nil {
		t.Fatalf("a valid channel: %v", err)
	}
	if _, err := w.svc.CreateChannel(ctx, w.owner, notify.ChannelSpec{Name: "ok", Kind: notify.KindSlack, Secret: "https://hooks.slack.com/services/T/B/y"}); code(err) != plxerr.ResourceExists {
		t.Fatalf("a duplicate name: %v", err)
	}
}

// SEC-105: a name that resolves to an internal address is refused when the
// connection is made, whatever the configured URL looked like.
func TestSRV063DialRefusesInternalAddress(t *testing.T) {
	cap := newCapture(t)
	// The policy that created the channel allowed loopback; the sender does not.
	w := newWorld(t, func(o *notify.Options) {
		o.HTTP = httpx.NewClient(httpx.Options{Policy: httpx.Policy{}, Timeout: 5 * time.Second})
	})
	ctx := context.Background()
	ch, err := w.svc.CreateChannel(ctx, w.owner, notify.ChannelSpec{Name: "s", Kind: notify.KindSlack, Secret: cap.srv.URL + "/hook"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.svc.TestChannel(ctx, w.owner, ch.ID); code(err) != plxerr.ChannelMisconfigured {
		t.Fatalf("TestChannel = %v, want PLX-8400", err)
	}
	if cap.count() != 0 {
		t.Fatal("the request reached a loopback address")
	}
}

// ---- SMTP ----------------------------------------------------------------

type smtpServer struct {
	ln       net.Listener
	cert     tls.Certificate
	pool     *x509.CertPool
	startTLS bool
	mu       sync.Mutex
	messages []string
	auth     []string
	clearTLS []bool
}

func newSMTPServer(t *testing.T, startTLS bool) *smtpServer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smtpServer{ln: ln, cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool: pool, startTLS: startTLS}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *smtpServer) port() string { _, p, _ := net.SplitHostPort(s.ln.Addr().String()); return p }

func (s *smtpServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	say := func(l string) { _, _ = conn.Write([]byte(l + "\r\n")) }
	encrypted := false
	say("220 localhost ESMTP")
	var data strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			say("250-localhost")
			if s.startTLS && !encrypted {
				say("250-STARTTLS")
			}
			say("250 AUTH PLAIN")
		case up == "STARTTLS":
			say("220 ready")
			tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
			if tc.Handshake() != nil {
				return
			}
			conn, r, encrypted = tc, bufio.NewReader(tc), true
		case strings.HasPrefix(up, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			s.mu.Lock()
			s.auth = append(s.auth, strings.ReplaceAll(string(raw), "\x00", "|"))
			s.clearTLS = append(s.clearTLS, !encrypted)
			s.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
			say("250 ok")
		case up == "DATA":
			say("354 go")
			data.Reset()
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(l)
			}
			s.mu.Lock()
			s.messages = append(s.messages, data.String())
			s.mu.Unlock()
			say("250 queued")
		case up == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

// SRV-062: mail goes over STARTTLS to each team member's address; a relay
// without STARTTLS is refused, and "none" works only where the installation
// allows insecure development settings.
func TestSRV062SMTP(t *testing.T) {
	good := newSMTPServer(t, true)
	plain := newSMTPServer(t, false)
	w := newWorld(t, func(o *notify.Options) { o.SMTPTLS = &tls.Config{RootCAs: good.pool, MinVersion: tls.VersionTLS12} })
	ctx := context.Background()
	mk := func(name string, srv *smtpServer, extra map[string]string) notify.ChannelSpec {
		settings := map[string]string{"host": "localhost", "port": srv.port(), "from": "Plux <plux@example.com>"}
		for k, v := range extra {
			settings[k] = v
		}
		return notify.ChannelSpec{Name: name, Kind: notify.KindSMTP, Settings: settings}
	}
	spec := mk("mail", good, map[string]string{"username": "plux"})
	spec.Secret = "smtp-password-123"
	mail, err := w.svc.CreateChannel(ctx, w.owner, spec)
	if err != nil {
		t.Fatal(err)
	}
	if mail.Settings["tls"] != "starttls" {
		t.Fatalf("tls defaults to %q, want starttls", mail.Settings["tls"])
	}
	if _, ok := mail.Settings["password"]; ok || strings.Contains(mail.Hint, "smtp-pass") {
		t.Fatalf("the password leaks: %+v", mail)
	}
	if _, err := w.svc.Subscribe(ctx, w.owner, notify.Subscription{SubjectKind: "team", SubjectID: w.team, EventType: notify.LockRequested, ChannelID: mail.ID}); err != nil {
		t.Fatal(err)
	}
	// A person may route their own mail through a mail channel.
	if _, err := w.svc.Subscribe(ctx, w.developer, notify.Subscription{EventType: notify.LockRequested, ChannelID: mail.ID}); err != nil {
		t.Fatalf("self-subscribe to mail: %v", err)
	}
	e := w.event(notify.LockRequested, "Lock wanted: Home page")
	e.Body = "Ana asked for the lock on Home — ünïcode\r\nBcc: attacker@example.com"
	if err := w.svc.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	if n, err := w.svc.DeliverDue(ctx, w.org); err != nil || n != 2 {
		t.Fatalf("DeliverDue = %d, %v; want one mail per member, the developer once", n, err)
	}
	good.mu.Lock()
	defer good.mu.Unlock()
	if len(good.messages) != 2 {
		t.Fatalf("%d messages", len(good.messages))
	}
	for _, c := range good.clearTLS {
		if c {
			t.Fatal("the password was sent before TLS")
		}
	}
	if len(good.auth) == 0 || good.auth[0] != "|plux|smtp-password-123" {
		t.Fatalf("auth = %v", good.auth)
	}
	msg := good.messages[0]
	for _, want := range []string{"From: \"Plux\" <plux@example.com>", "Subject: [Plux] Lock wanted: Home page", "Content-Transfer-Encoding: quoted-printable", "MIME-Version: 1.0"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if head, _, _ := strings.Cut(msg, "\r\n\r\n"); strings.Contains(head, "Bcc:") {
		t.Errorf("the body injected a header:\n%s", msg)
	}

	// A relay without STARTTLS is refused: nothing is sent in the clear.
	good.mu.Unlock()
	nostart, err := w.svc.CreateChannel(ctx, w.owner, mk("plain", plain, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.svc.TestChannel(ctx, w.owner, nostart.ID); code(err) != plxerr.ChannelMisconfigured || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("a relay without STARTTLS: %v", err)
	}
	plain.mu.Lock()
	if len(plain.messages) != 0 {
		t.Fatal("mail was sent without TLS")
	}
	plain.mu.Unlock()
	good.mu.Lock()

	// Explicitly disabled for development, it sends.
	clear, err := w.svc.CreateChannel(ctx, w.owner, mk("clear", plain, map[string]string{"tls": "none"}))
	if err != nil {
		t.Fatalf("tls=none in development: %v", err)
	}
	if err := w.svc.TestChannel(ctx, w.owner, clear.ID); err != nil {
		t.Fatalf("tls=none: %v", err)
	}
	plain.mu.Lock()
	defer plain.mu.Unlock()
	if len(plain.messages) != 1 {
		t.Fatalf("%d messages on the plain relay", len(plain.messages))
	}
}

// SEC-092: tls=none is refused unless the installation allows insecure
// development settings.
func TestSRV062SMTPNoTLSNeedsDevelopment(t *testing.T) {
	w := newWorld(t, func(o *notify.Options) { o.AllowInsecure = false })
	_, err := w.svc.CreateChannel(context.Background(), w.owner, notify.ChannelSpec{
		Name: "x", Kind: notify.KindSMTP, Settings: map[string]string{"host": "mail.example.com", "from": "a@example.com", "tls": "none"},
	})
	if code(err) != plxerr.ChannelMisconfigured {
		t.Fatalf("tls=none outside development: %v", err)
	}
}

// ---- outbound webhooks ---------------------------------------------------

// SRV-063: a webhook receives a message signed per Standard Webhooks that a
// receiver verifies; the id is stable across retries, the signature is not;
// rotation keeps both secrets valid for the overlap.
func TestSRV063OutboundWebhook(t *testing.T) {
	cap := newCapture(t)
	w := newWorld(t, nil)
	ctx := context.Background()

	if _, _, err := w.svc.CreateEndpoint(ctx, w.developer, notify.EndpointSpec{Name: "x", URL: cap.srv.URL}); code(err) != plxerr.PermissionDenied {
		t.Fatalf("a developer created a webhook: %v", err)
	}
	ep, secret, err := w.svc.CreateEndpoint(ctx, w.owner, notify.EndpointSpec{Name: "ci", URL: cap.srv.URL + "/in", EventTypes: []notify.EventType{notify.ReleasePublished}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "whsec_") || ep.SecretHint == "" || strings.Contains(ep.SecretHint, "whsec") {
		t.Fatalf("secret %q hint %q", secret, ep.SecretHint)
	}
	// An endpoint that follows another event type gets nothing.
	if err := w.svc.Publish(ctx, w.event(notify.AlertFired, "unrelated")); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "Release 7")); err != nil {
		t.Fatal(err)
	}
	if n, err := w.svc.DeliverDue(ctx, w.org); err != nil || n != 1 {
		t.Fatalf("DeliverDue = %d, %v", n, err)
	}
	req, header := cap.last(t)
	if req.path != "/in" {
		t.Fatalf("path %s", req.path)
	}
	msg, err := webhook.Verify([]string{secret}, header, []byte(req.body), w.now(), 0)
	if err != nil {
		t.Fatalf("the receiver cannot verify the message: %v", err)
	}
	if !strings.HasPrefix(msg.ID, "msg_") {
		t.Errorf("webhook-id %q", msg.ID)
	}
	for _, want := range []string{`"type":"release.published"`, `"timestamp":"`, `"title":"Release 7"`, `"attributes":{"release":"7"}`} {
		if !strings.Contains(req.body, want) {
			t.Errorf("body lacks %s: %s", want, req.body)
		}
	}
	if strings.Contains(req.body, secret) {
		t.Error("the body holds the secret")
	}

	// A failing receiver: retried with the same id and a fresh timestamp.
	cap.status.Store(503)
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "Release 8")); err != nil {
		t.Fatal(err)
	}
	w.advance(time.Second)
	if _, err := w.svc.DeliverDue(ctx, w.org); err != nil {
		t.Fatal(err)
	}
	_, first := cap.last(t)
	cap.status.Store(200)
	w.advance(time.Minute)
	if n, err := w.svc.DeliverDue(ctx, w.org); err != nil || n != 1 {
		t.Fatalf("retry = %d, %v", n, err)
	}
	_, second := cap.last(t)
	if first.Get(webhook.HeaderID) != second.Get(webhook.HeaderID) {
		t.Errorf("the id changed between attempts: %s %s", first.Get(webhook.HeaderID), second.Get(webhook.HeaderID))
	}
	if first.Get(webhook.HeaderTimestamp) == second.Get(webhook.HeaderTimestamp) {
		t.Error("the timestamp did not change between attempts")
	}

	// Rotation: both secrets verify during the overlap; the old one stops after.
	fresh, err := w.svc.RotateEndpointSecret(ctx, w.owner, ep.ID, time.Hour)
	if err != nil || fresh == secret {
		t.Fatalf("rotate = %q, %v", fresh, err)
	}
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "Release 9")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.DeliverDue(ctx, w.org); err != nil {
		t.Fatal(err)
	}
	req, header = cap.last(t)
	if n := len(strings.Fields(header.Get(webhook.HeaderSignature))); n != 2 {
		t.Fatalf("%d signatures during the overlap, want 2", n)
	}
	for name, s := range map[string]string{"old": secret, "new": fresh} {
		if _, err := webhook.Verify([]string{s}, header, []byte(req.body), w.now(), 0); err != nil {
			t.Errorf("the %s secret does not verify during the overlap: %v", name, err)
		}
	}
	w.advance(2 * time.Hour)
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "Release 10")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.DeliverDue(ctx, w.org); err != nil {
		t.Fatal(err)
	}
	req, header = cap.last(t)
	if n := len(strings.Fields(header.Get(webhook.HeaderSignature))); n != 1 {
		t.Fatalf("%d signatures after the overlap, want 1", n)
	}
	if _, err := webhook.Verify([]string{secret}, header, []byte(req.body), w.now(), 0); code(err) != plxerr.WebhookSignatureInvalid {
		t.Errorf("the old secret still verifies: %v", err)
	}
	if _, err := webhook.Verify([]string{fresh}, header, []byte(req.body), w.now(), 0); err != nil {
		t.Errorf("the new secret: %v", err)
	}
	if n, err := w.svc.Purge(ctx, w.org); err != nil || n != 1 {
		t.Errorf("Purge = %d, %v; want the expired secret", n, err)
	}

	list, err := w.svc.ListEndpoints(ctx, w.owner)
	if err != nil || len(list) != 1 || list[0].URL != ep.URL {
		t.Fatalf("ListEndpoints = %+v, %v", list, err)
	}
	if err := w.svc.DeleteEndpoint(ctx, w.owner, ep.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.DeleteEndpoint(ctx, w.owner, ep.ID); code(err) != plxerr.ResourceNotFound {
		t.Fatalf("second delete: %v", err)
	}
}

// SRV-063: a receiver that never answers is retried for 24 hours, then the
// delivery is given up on with PLX-8401 and audited.
func TestSRV063GivesUpAfter24Hours(t *testing.T) {
	cap := newCapture(t)
	cap.status.Store(500)
	w := newWorld(t, nil)
	ctx := context.Background()
	ep, _, err := w.svc.CreateEndpoint(ctx, w.owner, notify.EndpointSpec{Name: "down", URL: cap.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.svc.Publish(ctx, w.event(notify.BreakGlassUsed, "Break-glass")); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	for range 200 {
		n, err := w.svc.DeliverDue(ctx, w.org)
		if err != nil {
			t.Fatal(err)
		}
		attempts += n
		log, _ := w.svc.Deliveries(ctx, w.owner, notify.DeliveryFilter{Kind: "webhook", TargetID: ep.ID})
		if len(log) == 1 && log[0].State == "failed" {
			break
		}
		w.advance(30 * time.Minute)
	}
	log, _ := w.svc.Deliveries(ctx, w.owner, notify.DeliveryFilter{Kind: "webhook", TargetID: ep.ID})
	if len(log) != 1 || log[0].State != "failed" || log[0].LastStatus != 500 || int(log[0].Attempts) != attempts {
		t.Fatalf("log = %+v after %d attempts", log, attempts)
	}
	if attempts < 8 || attempts > 40 {
		t.Errorf("%d attempts in 24 hours", attempts)
	}
	if since := w.now().Sub(log[0].CreatedAt); since < 20*time.Hour || since > 26*time.Hour {
		t.Errorf("gave up after %s", since)
	}
	if countAudit(t, w, audit.DeliveryFailed) != 1 {
		t.Error("the failure is not audited")
	}
	// A given-up delivery is not retried.
	before := cap.count()
	w.advance(48 * time.Hour)
	if n, _ := w.svc.DeliverDue(ctx, w.org); n != 0 || cap.count() != before {
		t.Error("a failed delivery was retried")
	}
}

// SRV-063: webhooks.perOrg bounds the endpoints, and the URL must be safe.
func TestSRV063EndpointLimitAndSSRF(t *testing.T) {
	w := newWorld(t, func(o *notify.Options) { o.Policy = httpx.Policy{}; o.AllowInsecure = false })
	ctx := context.Background()
	set, err := limits.Defaults().Tighten(limits.WebhooksPerOrg, limits.ScopeOrganization, 2)
	if err != nil {
		t.Fatal(err)
	}
	w.limits.Store(&set)
	for _, u := range []string{"https://127.0.0.1/x", "https://[::1]/x", "https://192.168.1.1/x", "http://example.com/x", "ftp://example.com/x", "https://169.254.169.254/x"} {
		if _, _, err := w.svc.CreateEndpoint(ctx, w.owner, notify.EndpointSpec{Name: "bad", URL: u}); code(err) != plxerr.ChannelMisconfigured {
			t.Errorf("%s: %v, want PLX-8400", u, err)
		}
	}
	for _, name := range []string{"a", "b"} {
		if _, _, err := w.svc.CreateEndpoint(ctx, w.owner, notify.EndpointSpec{Name: name, URL: "https://example.com/" + name}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := w.svc.CreateEndpoint(ctx, w.owner, notify.EndpointSpec{Name: "c", URL: "https://example.com/c"}); code(err) != plxerr.LimitExceeded {
		t.Fatalf("the third endpoint: %v, want webhooks.perOrg", err)
	}
	if _, _, err := w.svc.CreateEndpoint(ctx, w.owner, notify.EndpointSpec{Name: "d", URL: "https://example.com/d", EventTypes: []notify.EventType{"made.up"}}); err == nil {
		t.Error("an unknown event type was accepted")
	}
}
