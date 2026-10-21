// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/notify/webhook"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// triggerClock is a clock the tests move.
type triggerClock struct{ unix atomic.Int64 }

func (c *triggerClock) now() time.Time { return time.Unix(c.unix.Load(), 0).UTC() }

func (c *triggerClock) advance(d time.Duration) { c.unix.Add(int64(d / time.Second)) }

// fakeInvoker records the server calls of triggers.
type fakeInvoker struct {
	mu    sync.Mutex
	calls []function.ServerCall
	err   error
	out   []byte
}

func (f *fakeInvoker) InvokeServer(_ context.Context, c function.ServerCall) (function.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	if f.err != nil {
		return function.Result{}, f.err
	}
	return function.Result{Output: f.out, Version: 1}, nil
}

func (f *fakeInvoker) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// recordingPublisher keeps the events it is given.
type recordingPublisher struct {
	mu     sync.Mutex
	events []notify.Event
}

func (r *recordingPublisher) Publish(_ context.Context, e notify.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *recordingPublisher) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// triggerWorld is a world with a deployed server function, a clock, an
// invoker and a notification service that keeps hook secrets.
type triggerWorld struct {
	*world
	clock     *triggerClock
	invoker   *fakeInvoker
	publisher *recordingPublisher
	secrets   *notify.Service
}

func newTriggerWorld(t *testing.T) *triggerWorld {
	t.Helper()
	w := newWorld(t, nil, nil)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := &triggerClock{}
	secrets, err := notify.New(notify.Options{
		DB: w.db, Audit: w.opts.Audit, Crypter: backend, IDs: ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}, Now: clock.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	tw := &triggerWorld{world: w, clock: clock, invoker: &fakeInvoker{out: []byte(`{"ok":true}`)}, publisher: &recordingPublisher{}, secrets: secrets}
	tw.clock.unix.Store(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).Unix())
	w.svc = tw.service(t)
	w.deployWith(t, "development", fnName, function.PlacementServer, `{"name":"payments.charge","placement":"server"}`, "live")
	return tw
}

// service returns a function service sharing the world's database, the
// clock and the invoker; each call is a separate replica.
func (tw *triggerWorld) service(t *testing.T) *function.Service {
	t.Helper()
	svc := tw.world.service(t, func(o *function.Options) {
		o.Now = tw.clock.now
		o.Secrets = tw.secrets
		o.Notify = tw.publisher
	})
	svc.UseInvoker(tw.invoker)
	return svc
}

func (tw *triggerWorld) spec(expression, zone string) function.TriggerSpec {
	return function.TriggerSpec{
		AppID: tw.app, EnvironmentID: tw.envs["development"], Function: fnName, Alias: "live", Expression: expression, Timezone: zone, Input: []byte(`{"n":1}`),
	}
}

// Verifies: FN-025.
// A schedule fires when due, once per fire time, and a worker that was down
// runs it once for the time it missed rather than once per missed minute.
func TestTriggerFiresOncePerFireTime_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	trig, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec("* * * * *", ""))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 1, 12, 1, 0, 0, time.UTC); !trig.NextFireAt.Equal(want) {
		t.Fatalf("next fire = %s, want %s", trig.NextFireAt, want)
	}
	if n, err := tw.svc.RunTriggers(ctx, tw.org); err != nil || n != 0 {
		t.Fatalf("before the fire time: %d %v", n, err)
	}
	tw.clock.advance(61 * time.Second)
	if n, err := tw.svc.RunTriggers(ctx, tw.org); err != nil || n != 1 {
		t.Fatalf("at the fire time: %d %v", n, err)
	}
	if n, _ := tw.svc.RunTriggers(ctx, tw.org); n != 0 {
		t.Errorf("the same fire time ran twice: %d", n)
	}
	call := tw.invoker.calls[0]
	if call.Function != fnName || call.Alias != "live" || string(call.Input) != `{"n":1}` || call.OrganizationID != tw.org {
		t.Errorf("call = %+v", call)
	}
	tw.clock.advance(3 * time.Hour)
	if n, _ := tw.svc.RunTriggers(ctx, tw.org); n != 1 {
		t.Errorf("a schedule 180 minutes behind ran %d times, want once", n)
	}
	list, err := tw.svc.ListTriggers(ctx, tw.viewer, tw.app)
	if err != nil || len(list) != 1 || list[0].LastResult != "ok" {
		t.Errorf("list = %+v, %v", list, err)
	}
}

// Verifies: FN-025.
// Replicas racing for the same due schedule run it once.
func TestTriggerReplicasRunOnce_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	if _, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec("*/5 * * * *", "")); err != nil {
		t.Fatal(err)
	}
	tw.clock.advance(10 * time.Minute)
	replicas := []*function.Service{tw.service(t), tw.service(t), tw.service(t)}
	var wg sync.WaitGroup
	for _, r := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.RunTriggers(ctx, tw.org); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := tw.invoker.count(); got != 1 {
		t.Errorf("%d replicas ran the schedule %d times, want once", len(replicas), got)
	}
}

// Verifies: FN-025.
// Bad expressions and zones, unknown aliases and device functions are
// refused, and creating a schedule needs function.deploy.
func TestTriggerCreateRefusals_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	for _, c := range []struct{ expr, zone string }{{"61 * * * *", ""}, {"* * *", ""}, {"@never", ""}, {"* * * * *", "Mars/Olympus"}} {
		if _, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec(c.expr, c.zone)); code(err) != plxerr.CronExpressionInvalid {
			t.Errorf("%q in %q: %v", c.expr, c.zone, err)
		}
	}
	if _, err := tw.svc.CreateTrigger(ctx, tw.viewer, tw.spec("@daily", "")); err == nil {
		t.Error("a viewer scheduled a function")
	}
	missing := tw.spec("@daily", "")
	missing.Alias = "nowhere"
	if _, err := tw.svc.CreateTrigger(ctx, tw.developer, missing); err == nil {
		t.Error("an alias that does not exist was scheduled")
	}
	tw.deployWith(t, "development", "device.only", function.PlacementDevice, `{"name":"device.only","placement":"device"}`, "live")
	onDevice := tw.spec("@daily", "")
	onDevice.Function = "device.only"
	if _, err := tw.svc.CreateTrigger(ctx, tw.developer, onDevice); err == nil {
		t.Error("a device function was scheduled on the server")
	}
}

// Verifies: FN-025, SRV-062.
// The first failure of a schedule publishes function.failing; the failures
// after it do not, and a recovery re-arms it.
func TestTriggerFailureNotifiesOnce_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	if _, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec("* * * * *", "")); err != nil {
		t.Fatal(err)
	}
	tw.invoker.err = plxerr.New(plxerr.FunctionNotFound, "gone")
	for range 3 {
		tw.clock.advance(time.Minute)
		if _, err := tw.svc.RunTriggers(ctx, tw.org); err != nil {
			t.Fatal(err)
		}
	}
	if got := tw.publisher.count(); got != 1 {
		t.Fatalf("%d failure events, want 1", got)
	}
	e := tw.publisher.events[0]
	if e.Type != notify.FunctionFailing || e.OrganizationID != tw.org || !strings.Contains(e.Body, "PLX-") {
		t.Errorf("event = %+v", e)
	}
	tw.invoker.err = nil
	tw.clock.advance(time.Minute)
	_, _ = tw.svc.RunTriggers(ctx, tw.org)
	tw.invoker.err = errors.New("again")
	tw.clock.advance(time.Minute)
	_, _ = tw.svc.RunTriggers(ctx, tw.org)
	if got := tw.publisher.count(); got != 2 {
		t.Errorf("%d failure events after a recovery, want 2", got)
	}
}

// Verifies: FN-025.
// A deleted schedule never fires.
func TestTriggerDelete_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	trig, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec("* * * * *", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.svc.DeleteTrigger(ctx, tw.developer, trig.ID); err != nil {
		t.Fatal(err)
	}
	tw.clock.advance(time.Hour)
	if n, _ := tw.svc.RunTriggers(ctx, tw.org); n != 0 {
		t.Errorf("a deleted schedule ran %d times", n)
	}
}

// hookRequest posts a body to the hook with the headers given.
func hookRequest(svc *function.Service, app, name string, body string, h http.Header) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("POST /hooks/{app}/{function}", svc.HookHandler())
	req := httptest.NewRequest(http.MethodPost, "/hooks/"+app+"/"+name, strings.NewReader(body))
	for k, v := range h {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// Verifies: FN-025.
// An inbound hook runs the function for a correctly signed message, and
// refuses a wrong signature, a replay, a stale timestamp and an unknown hook.
func TestHookVerifiesAndRejectsReplay_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	hook, secret, err := tw.svc.CreateHook(ctx, tw.developer, function.HookSpec{AppID: tw.app, EnvironmentID: tw.envs["development"], Function: fnName, Alias: "live"})
	if err != nil || !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("hook = %+v, secret %q, %v", hook, secret, err)
	}
	body := `{"event":"paid"}`
	sign := func(id string, at time.Time, secret string) http.Header {
		h, err := webhook.Headers([]string{secret}, id, at, []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	now := tw.clock.now()

	rec := hookRequest(tw.svc, tw.app, fnName, body, sign("msg_1", now, secret))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"ok":true}` {
		t.Fatalf("a signed message: %d %s", rec.Code, rec.Body)
	}
	if got := string(tw.invoker.calls[0].Input); got != body {
		t.Errorf("input = %q", got)
	}
	if rec := hookRequest(tw.svc, tw.app, fnName, body, sign("msg_1", now, secret)); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "PLX-7152") {
		t.Errorf("a replay: %d %s", rec.Code, rec.Body)
	}
	other, err := webhook.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if rec := hookRequest(tw.svc, tw.app, fnName, body, sign("msg_2", now, other)); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "PLX-7151") {
		t.Errorf("a wrong secret: %d %s", rec.Code, rec.Body)
	}
	if rec := hookRequest(tw.svc, tw.app, fnName, body+" ", sign("msg_3", now, secret)); rec.Code != http.StatusUnauthorized {
		t.Errorf("a tampered body: %d", rec.Code)
	}
	if rec := hookRequest(tw.svc, tw.app, fnName, body, sign("msg_4", now.Add(-time.Hour), secret)); rec.Code != http.StatusConflict {
		t.Errorf("a stale timestamp: %d %s", rec.Code, rec.Body)
	}
	if rec := hookRequest(tw.svc, tw.app, fnName, body, http.Header{}); rec.Code != http.StatusUnauthorized {
		t.Errorf("an unsigned message: %d", rec.Code)
	}
	if rec := hookRequest(tw.svc, tw.app, "no.such.fn", body, sign("msg_5", now, secret)); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown hook: %d", rec.Code)
	}
	if got := tw.invoker.count(); got != 1 {
		t.Errorf("the function ran %d times, want once", got)
	}
}

// Verifies: FN-025, SRV-063.
// After a rotation both secrets verify during the overlap; afterwards only
// the new one does, and a rotation ended at once leaves only the new one.
func TestHookSecretRotation_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	_, oldSecret, err := tw.svc.CreateHook(ctx, tw.developer, function.HookSpec{AppID: tw.app, EnvironmentID: tw.envs["development"], Function: fnName, Alias: "live"})
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := tw.svc.ListHooks(ctx, tw.viewer, tw.app)
	if err != nil || len(hooks) != 1 {
		t.Fatalf("hooks = %+v, %v", hooks, err)
	}
	newSecret, err := tw.svc.RotateHookSecret(ctx, tw.developer, hooks[0].ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	post := func(id, secret string) int {
		h, err := webhook.Headers([]string{secret}, id, tw.clock.now(), []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
		return hookRequest(tw.svc, tw.app, fnName, "{}", h).Code
	}
	if post("a", oldSecret) != http.StatusOK || post("b", newSecret) != http.StatusOK {
		t.Error("both secrets must verify during the overlap")
	}
	tw.clock.advance(2 * time.Hour)
	if post("c", oldSecret) != http.StatusUnauthorized || post("d", newSecret) != http.StatusOK {
		t.Error("only the new secret verifies after the overlap")
	}
}
