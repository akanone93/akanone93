// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// fakeConfig is the database side of the configuration: it holds the
// values, counts the loads, and announces changes in the cache store like
// Configs does.
type fakeConfig struct {
	mu                            sync.Mutex
	epoch, liveEpoch, strictEpoch int64
	values                        map[string]StoredValue
	loads, epochs                 int
	cache                         cache.Cache
	app, env                      string
}

func newFakeConfig(c cache.Cache) *fakeConfig {
	return &fakeConfig{values: map[string]StoredValue{}, cache: c, app: "app-1", env: "env-1"}
}

func (c *fakeConfig) Epoch(context.Context, string, string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epochs++
	return c.epoch, nil
}

func (c *fakeConfig) Load(context.Context, string, string) (StoredConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loads++
	out := StoredConfig{Epoch: c.epoch, LiveEpoch: c.liveEpoch, StrictEpoch: c.strictEpoch}
	for _, v := range c.values {
		out.Values = append(out.Values, v)
	}
	return out, nil
}

// set changes a value and announces the new epoch.
func (c *fakeConfig) set(t testing.TB, v StoredValue) {
	t.Helper()
	c.mu.Lock()
	c.epoch++
	if v.Tier == TierLive {
		c.liveEpoch = c.epoch
	}
	if v.Strict {
		c.strictEpoch = c.epoch
	}
	v.Version = c.epoch
	c.values[v.Key] = v
	epoch := c.epoch
	c.mu.Unlock()
	if err := c.cache.Set(context.Background(), EpochKey(c.app, c.env), []byte(strconv.FormatInt(epoch, 10)), epochTTL); err != nil {
		t.Fatal(err)
	}
}

func bound(key, text string) StoredValue {
	return StoredValue{Key: key, Tier: TierServerBound, Type: TypeString, Text: text}
}

// withConfig gives the fixture a configuration and a bank operation "pay"
// that binds poolAccount into the body, a header, the query and the path.
func (f *fixture) withConfig() *fakeConfig {
	f.t.Helper()
	src := newFakeConfig(f.cache)
	f.config = &ConfigResolver{Source: src, Cache: f.cache, Now: f.clock.Now}
	up := f.ups["bank"]
	up.Operations = append(up.Operations,
		Operation{
			Name: "pay", Methods: []string{"POST"}, PathPrefix: "/payments",
			Bindings: []Binding{{Key: "poolAccount", Location: BindJSONPointer, Name: "/creditor/account"}},
		},
		Operation{
			Name: "lookup", Methods: []string{"GET"}, PathPrefix: "/providers",
			Bindings: []Binding{
				{Key: "providerId", Location: BindPathParam, Name: "provider"},
				{Key: "poolAccount", Location: BindHeader, Name: "X-Account"},
				{Key: "region", Location: BindQueryParam, Name: "region"},
			},
		},
	)
	f.ups["bank"] = up
	f.build()
	return src
}

// Verifies: DAT-091.
// A bound value is put into the body at its JSON Pointer, the header, the
// query and the path the operation declares; numbers the device wrote keep
// their text.
func TestBindingsAreInjected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, bound("poolAccount", "POOL-42"))
	src.set(t, bound("providerId", "acme"))
	src.set(t, bound("region", "eu west"))

	if _, err := f.call(Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}, []byte(`{"amount":12345678901234567890.10,"note":"<a&b>"}`)); err != nil {
		t.Fatalf("pay: %v", err)
	}
	got := f.next()
	want := `{"amount":12345678901234567890.10,"creditor":{"account":"POOL-42"},"note":"<a&b>"}`
	if string(got.Body) != want {
		t.Errorf("body = %s, want %s", got.Body, want)
	}

	if _, err := f.call(Request{Upstream: "bank", Operation: "lookup", Method: "GET", Target: "/providers/fees?x=1"}, nil); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	got = f.next()
	if got.URI != "/api/providers/acme/fees?x=1&region=eu+west" {
		t.Errorf("uri = %s", got.URI)
	}
	if got.Header.Get("X-Account") != "POOL-42" {
		t.Errorf("header = %q", got.Header.Get("X-Account"))
	}
}

// Verifies: DAT-091.
// A request that sets a bound location is refused, and the upstream hears
// nothing; an empty body is enough for an operation whose body is bound.
func TestDeviceCannotSetBoundLocations(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, bound("poolAccount", "POOL-42"))
	src.set(t, bound("providerId", "acme"))
	src.set(t, bound("region", "eu"))

	pay := Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}
	lookup := Request{Upstream: "bank", Operation: "lookup", Method: "GET", Target: "/providers"}
	withHeader := lookup
	withHeader.Header = http.Header{"x-account": {"mine"}}
	withQuery := lookup
	withQuery.Target = "/providers?region=mine"
	tests := []struct {
		name string
		req  Request
		body string
	}{
		{"the member", pay, `{"creditor":{"account":"MINE"}}`},
		{"a value where an object is needed", pay, `{"creditor":"x"}`},
		{"the header", withHeader, ""},
		{"the query", withQuery, ""},
	}
	for _, tc := range tests {
		var body []byte
		if tc.body != "" {
			body = []byte(tc.body)
		}
		if _, err := f.call(tc.req, body); code(err) != plxerr.BoundLocationSetByDevice {
			t.Errorf("%s: %v, want PLX-6065", tc.name, err)
		}
	}
	f.none()
	if _, err := f.call(pay, nil); err != nil {
		t.Errorf("an empty body: %v", err)
	}
	if got := f.next(); string(got.Body) != `{"creditor":{"account":"POOL-42"}}` {
		t.Errorf("body = %s", got.Body)
	}
	if _, err := f.call(pay, []byte(`[1]`)); code(err) != plxerr.InvalidFormat {
		t.Errorf("a body that is not an object: %v", err)
	}
}

// Verifies: DAT-091.
// An operation without bindings asks for no body, so the route streams it
// untouched; one with a bound member asks for it.
func TestUnboundOperationsStream(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, bound("poolAccount", "POOL-1"))
	for _, tc := range []struct {
		req  Request
		want bool
	}{
		{Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}, false},
		{Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}, true},
	} {
		p, err := f.gw.Begin(context.Background(), f.dev, tc.req)
		if err != nil {
			t.Fatalf("%s: %v", tc.req.Operation, err)
		}
		if p.NeedsBody() != tc.want {
			t.Errorf("%s: NeedsBody = %v, want %v", tc.req.Operation, p.NeedsBody(), tc.want)
		}
	}
}

// Verifies: DAT-092.
// The values are loaded once per epoch; a change moves the epoch and the
// next call sees the new value, with no wait.
func TestChangesAreSeenAtOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, bound("poolAccount", "POOL-1"))
	pay := Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}
	post := func() string {
		t.Helper()
		if _, err := f.call(pay, nil); err != nil {
			t.Fatalf("pay: %v", err)
		}
		return string(f.next().Body)
	}
	if got := post(); !strings.Contains(got, "POOL-1") {
		t.Errorf("body = %s", got)
	}
	post()
	if src.loads != 1 {
		t.Errorf("%d loads for two calls at one epoch, want 1", src.loads)
	}
	src.set(t, bound("poolAccount", "POOL-2"))
	if got := post(); !strings.Contains(got, "POOL-2") {
		t.Errorf("a changed value was not used at once: %s", got)
	}
	if src.loads != 2 {
		t.Errorf("%d loads, want 2", src.loads)
	}
}

// Verifies: DAT-092.
// A key that the cache store lost is seeded from the database, and a store
// that lost a write is healed from it.
func TestEpochKeyIsSeeded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, bound("poolAccount", "POOL-1"))
	if err := f.cache.Delete(context.Background(), EpochKey("app-1", "env-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call(Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}, nil); err != nil {
		t.Fatalf("pay after the key was lost: %v", err)
	}
	f.next()
	raw, found, _ := f.cache.Get(context.Background(), EpochKey("app-1", "env-1"))
	if !found || string(raw) != "1" {
		t.Errorf("epoch key = %q, %v", raw, found)
	}
	// The store lost the latest write: the database is ahead. A replica
	// notices when its copy reaches configMaxAge.
	src.set(t, bound("poolAccount", "POOL-2"))
	if err := f.cache.Set(context.Background(), EpochKey("app-1", "env-1"), []byte("1"), epochTTL); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(configMaxAge)
	if _, err := f.call(Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}, nil); err != nil {
		t.Fatal(err)
	}
	f.next()
	raw, _, _ = f.cache.Get(context.Background(), EpochKey("app-1", "env-1"))
	if string(raw) != "2" {
		t.Errorf("epoch key = %q, want it healed to 2", raw)
	}
}

// brokenCache fails reads of its keys.
type brokenCache struct {
	cache.Cache
	mu   sync.Mutex
	down bool
}

func (b *brokenCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b.mu.Lock()
	down := b.down
	b.mu.Unlock()
	if down {
		return nil, false, errors.New("down")
	}
	return b.Cache.Get(ctx, key) //nolint:wrapcheck // a test double
}

func (b *brokenCache) fail() {
	b.mu.Lock()
	b.down = true
	b.mu.Unlock()
}

// Verifies: DAT-092.
// A call whose epoch cannot be checked is refused, however fresh the
// replica's copy is; an operation that needs no configuration still goes.
func TestFailsClosedWhenTheEpochCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, bound("poolAccount", "POOL-1"))
	broken := &brokenCache{Cache: f.cache}
	f.config = &ConfigResolver{Source: src, Cache: broken, Now: f.clock.Now}
	f.build()
	pay := Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}
	if _, err := f.call(pay, nil); err != nil {
		t.Fatalf("pay: %v", err)
	}
	f.next()
	broken.fail()
	if _, err := f.call(pay, nil); code(err) != plxerr.ConfigStoreUnavailable {
		t.Errorf("pay with the cache store down: %v, want PLX-6069", err)
	}
	v := int64(1)
	balance := Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", ConfigVersion: &v}
	if _, err := f.call(balance, nil); code(err) != plxerr.ConfigStoreUnavailable {
		t.Errorf("a call with a configuration version: %v, want PLX-6069", err)
	}
	balance.ConfigVersion = nil
	if _, err := f.call(balance, nil); err != nil {
		t.Errorf("a call that needs no configuration: %v", err)
	}
	f.next()
	f.none()
}

// Verifies: DAT-092.
// A gateway with no configuration refuses an operation that binds values.
func TestBindingsNeedConfiguration(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.withConfig()
	f.config = nil
	f.build()
	if _, err := f.call(Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}, nil); code(err) != plxerr.ConfigStoreUnavailable {
		t.Errorf("err = %v", err)
	}
}

// Verifies: DAT-091.
// Only a value that is server-bound and set can be bound.
func TestOnlyServerBoundValuesAreBound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	pay := Request{Upstream: "bank", Operation: "pay", Method: "POST", Target: "/payments"}
	if _, err := f.call(pay, nil); code(err) != plxerr.PreconditionFailed {
		t.Errorf("an unset value: %v", err)
	}
	src.set(t, StoredValue{Key: "poolAccount", Tier: TierLive, Type: TypeString, Text: "x"})
	if _, err := f.call(pay, nil); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a live value: %v", err)
	}
	f.none()
}

// Verifies: DAT-095, DAT-096.
// A call made with an older live version than a strict change is refused
// with PLX-6067; one made with the current version, or none, goes through;
// every call learns the epoch.
func TestStrictLiveChangesRefuseOlderVersions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.withConfig()
	src.set(t, StoredValue{Key: "limit", Tier: TierLive, Type: TypeInteger, Text: "10"})
	src.set(t, StoredValue{Key: "feeRate", Tier: TierLive, Type: TypeDecimal, Text: "0.015", Strict: true})
	src.set(t, StoredValue{Key: "banner", Tier: TierLive, Type: TypeString, Text: "hi"})
	src.set(t, bound("poolAccount", "POOL-1"))
	balance := func(version *int64) (*Pending, error) {
		return f.gw.Begin(context.Background(), f.dev, Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", ConfigVersion: version})
	}
	older, current := int64(1), int64(4)
	if _, err := balance(&older); code(err) != plxerr.LiveConfigStale {
		t.Errorf("an older version: %v, want PLX-6067", err)
	}
	p, err := balance(&current)
	if err != nil {
		t.Fatalf("the current version: %v", err)
	}
	if epoch, ok := p.ConfigEpoch(); !ok || epoch != 4 {
		t.Errorf("epoch = %d, %v", epoch, ok)
	}
	// A strict change after the device's version, but a version at or past it passes.
	src.set(t, StoredValue{Key: "feeRate", Tier: TierLive, Type: TypeDecimal, Text: "0.02", Strict: true})
	if _, err := balance(&current); code(err) != plxerr.LiveConfigStale {
		t.Errorf("after another strict change: %v", err)
	}
	fresh := int64(5)
	if _, err := balance(&fresh); err != nil {
		t.Errorf("after refreshing: %v", err)
	}
	// A change of a value that is not strict does not refuse anything.
	src.set(t, StoredValue{Key: "banner", Tier: TierLive, Type: TypeString, Text: "new"})
	if _, err := balance(&fresh); err != nil {
		t.Errorf("a non-strict change: %v", err)
	}
	if p, err = balance(nil); err != nil {
		t.Fatalf("no version: %v", err)
	}
	if epoch, ok := p.ConfigEpoch(); !ok || epoch != 6 {
		t.Errorf("epoch without a version = %d, %v", epoch, ok)
	}
	if epoch, ok := f.gw.ConfigEpoch(context.Background(), f.dev); !ok || epoch != 6 {
		t.Errorf("ConfigEpoch = %d, %v", epoch, ok)
	}
}

// transferBound registers "payout", a transaction whose payee is a
// displayed server-held value.
func (f *fixture) transferBound() *fakeConfig {
	f.t.Helper()
	src := f.withConfig()
	up := f.ups["bank"]
	up.Operations = append(up.Operations, Operation{
		Name: "payout", Methods: []string{"POST"}, PathPrefix: "/payouts", Assurance: "AL2",
		Transaction: &Transaction{Amount: "/amount", Currency: "/currency", Payee: "/to/iban"},
		Bindings:    []Binding{{Key: "payeeIban", Location: BindJSONPointer, Name: "/to/iban", Display: true, Label: "Payee"}},
	})
	f.ups["bank"] = up
	f.build()
	return src
}

const payoutSummary = `{"amount":"20.00","currency":"EUR"}`

// Verifies: DAT-093.
// The displayed value is resolved when the challenge is created, returned
// for the confirmation sheet, merged into the signed summary, and put into
// the body at execution.
func TestDisplayedValuesAreSignedAndInjected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.transferBound()
	src.set(t, bound("payeeIban", "DE89370400440532013000"))
	c, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "payout", []byte(payoutSummary))
	if err != nil {
		t.Fatalf("CreateChallenge: %v", err)
	}
	if len(c.Displayed) != 1 || c.Displayed[0] != (DisplayedValue{Label: "Payee", Value: "DE89370400440532013000"}) {
		t.Errorf("displayed = %v", c.Displayed)
	}
	const merged = `{"amount":"20.00","currency":"EUR","payee":"DE89370400440532013000"}`
	if string(c.Summary) != merged {
		t.Errorf("summary = %s, want %s", c.Summary, merged)
	}
	req := Request{
		Upstream: "bank", Operation: "payout", Method: "POST", Target: "/payouts", ChallengeID: c.ID,
		Header: http.Header{"Content-Type": {"application/json"}}, Signature: f.signTransaction(c, c.Summary),
	}
	if _, err := f.call(req, []byte(`{"amount":"20.00","currency":"EUR"}`)); err != nil {
		t.Fatalf("payout: %v", err)
	}
	if got := f.next(); string(got.Body) != `{"amount":"20.00","currency":"EUR","to":{"iban":"DE89370400440532013000"}}` {
		t.Errorf("body = %s", got.Body)
	}
}

// Verifies: DAT-093.
// If a displayed value changes between the challenge and the execution the
// call is refused with PLX-6066 and the challenge is not spent; a device
// that puts the bound member in its own summary or body is refused too.
func TestDisplayedValueChangeRefusesTheTransaction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.transferBound()
	src.set(t, bound("payeeIban", "DE89370400440532013000"))
	c, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "payout", []byte(payoutSummary))
	if err != nil {
		t.Fatal(err)
	}
	req := Request{
		Upstream: "bank", Operation: "payout", Method: "POST", Target: "/payouts", ChallengeID: c.ID,
		Header: http.Header{"Content-Type": {"application/json"}}, Signature: f.signTransaction(c, c.Summary),
	}
	src.set(t, bound("payeeIban", "DE00000000000000000000"))
	if _, err := f.call(req, []byte(`{"amount":"20.00","currency":"EUR"}`)); code(err) != plxerr.BoundValueChanged {
		t.Fatalf("after the change: %v, want PLX-6066", err)
	}
	f.none()
	// The device challenges again and succeeds.
	c2, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "payout", []byte(payoutSummary))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(c2.Summary), "DE00000000000000000000") {
		t.Errorf("summary = %s", c2.Summary)
	}
	req.ChallengeID, req.Signature = c2.ID, f.signTransaction(c2, c2.Summary)
	if _, err := f.call(req, []byte(`{"amount":"20.00","currency":"EUR"}`)); err != nil {
		t.Fatalf("after challenging again: %v", err)
	}
	f.next()

	own := `{"amount":"20.00","currency":"EUR","payee":"MINE"}`
	if _, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "payout", []byte(own)); code(err) != plxerr.BoundLocationSetByDevice {
		t.Errorf("a summary with the bound payee: %v", err)
	}
	c3, _ := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "payout", []byte(payoutSummary))
	req.ChallengeID, req.Signature = c3.ID, f.signTransaction(c3, c3.Summary)
	if _, err := f.call(req, []byte(`{"amount":"20.00","currency":"EUR","to":{"iban":"MINE"}}`)); code(err) != plxerr.BoundLocationSetByDevice {
		t.Errorf("a body with the bound payee: %v", err)
	}
	f.none()
}

// Verifies: DAT-093, DAT-090.
// A sensitive value is never shown to the user.
func TestSensitiveValuesAreNotDisplayed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	src := f.transferBound()
	v := bound("payeeIban", "DE89370400440532013000")
	v.Sensitive = true
	src.set(t, v)
	if _, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "payout", []byte(payoutSummary)); code(err) != plxerr.PreconditionFailed {
		t.Errorf("err = %v", err)
	}
}

// Verifies: DAT-091.
// Bindings are checked when an upstream is registered: where they may sit,
// what a displayed one needs, and that none overlaps another.
func TestBindingsAreValidated(t *testing.T) {
	t.Parallel()
	base := func(op Operation) Upstream {
		return Upstream{
			Name: "bank", BaseURL: "https://bank.example", Auth: Auth{Kind: AuthNone}, Methods: []string{"GET", "POST"},
			Operations: []Operation{op},
		}
	}
	tx := &Transaction{Amount: "/amount", Currency: "/currency", Payee: "/payee"}
	ptr := func(name string) Binding { return Binding{Key: "k", Location: BindJSONPointer, Name: name} }
	tests := []struct {
		name string
		op   Operation
		ok   bool
	}{
		{"pointer", Operation{Name: "o", Methods: []string{"POST"}, Bindings: []Binding{ptr("/a/b")}}, true},
		{"pointer on a GET", Operation{Name: "o", Methods: []string{"GET"}, Bindings: []Binding{ptr("/a")}}, false},
		{"root pointer", Operation{Name: "o", Methods: []string{"POST"}, Bindings: []Binding{ptr("")}}, false},
		{"nested pointers", Operation{Name: "o", Methods: []string{"POST"}, Bindings: []Binding{ptr("/a"), ptr("/a/b")}}, false},
		{"the same pointer twice", Operation{Name: "o", Methods: []string{"POST"}, Bindings: []Binding{ptr("/a"), ptr("/a")}}, false},
		{"header", Operation{Name: "o", Bindings: []Binding{{Key: "k", Location: BindHeader, Name: "X-Account"}}}, true},
		{"reserved header", Operation{Name: "o", Bindings: []Binding{{Key: "k", Location: BindHeader, Name: "Plux-Assertion"}}}, false},
		{"query", Operation{Name: "o", Bindings: []Binding{{Key: "k", Location: BindQueryParam, Name: "region"}}}, true},
		{"query with an ampersand", Operation{Name: "o", Bindings: []Binding{{Key: "k", Location: BindQueryParam, Name: "a&b"}}}, false},
		{"path parameter", Operation{Name: "o", PathPrefix: "/p", Bindings: []Binding{{Key: "k", Location: BindPathParam, Name: "id"}}}, true},
		{"path parameter without a prefix", Operation{Name: "o", Bindings: []Binding{{Key: "k", Location: BindPathParam, Name: "id"}}}, false},
		{"no location", Operation{Name: "o", Bindings: []Binding{{Key: "k"}}}, false},
		{"a bad key", Operation{Name: "o", Bindings: []Binding{{Key: "1x", Location: BindHeader, Name: "X-A"}}}, false},
		{"displayed payee", Operation{Name: "o", Methods: []string{"POST"}, Transaction: tx, Bindings: []Binding{{Key: "k", Location: BindJSONPointer, Name: "/payee", Display: true}}}, true},
		{"displayed elsewhere", Operation{Name: "o", Methods: []string{"POST"}, Transaction: tx, Bindings: []Binding{{Key: "k", Location: BindJSONPointer, Name: "/memo", Display: true}}}, false},
		{"displayed outside a transaction", Operation{Name: "o", Methods: []string{"POST"}, Bindings: []Binding{{Key: "k", Location: BindJSONPointer, Name: "/payee", Display: true}}}, false},
		{"displayed header", Operation{Name: "o", Methods: []string{"POST"}, Transaction: tx, Bindings: []Binding{{Key: "k", Location: BindHeader, Name: "X-A", Display: true}}}, false},
	}
	for _, tc := range tests {
		err := base(tc.op).Validate()
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok = %v", tc.name, err, tc.ok)
		}
	}
}

// Verifies: DAT-090.
// A value is checked against its tier and type before it is stored.
func TestConfigValueValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		v    ConfigValue
		ok   bool
	}{
		{"string", ConfigValue{Key: "a", Tier: TierServerBound, Type: TypeString, Value: "x"}, true},
		{"sensitive server-bound", ConfigValue{Key: "a", Tier: TierServerBound, Type: TypeString, Sensitive: true, Value: "x"}, true},
		{"sensitive live", ConfigValue{Key: "a", Tier: TierLive, Type: TypeString, Sensitive: true, Value: "x"}, false},
		{"strict live", ConfigValue{Key: "a", Tier: TierLive, Type: TypeString, Strict: true, Value: "x"}, true},
		{"strict server-bound", ConfigValue{Key: "a", Tier: TierServerBound, Type: TypeString, Strict: true, Value: "x"}, false},
		{"no tier", ConfigValue{Key: "a", Type: TypeString}, false},
		{"integer", ConfigValue{Key: "a", Tier: TierLive, Type: TypeInteger, Value: "-12"}, true},
		{"integer with zeros", ConfigValue{Key: "a", Tier: TierLive, Type: TypeInteger, Value: "012"}, false},
		{"integer too large", ConfigValue{Key: "a", Tier: TierLive, Type: TypeInteger, Value: "9223372036854775808"}, false},
		{"decimal", ConfigValue{Key: "a", Tier: TierLive, Type: TypeDecimal, Value: "0.015"}, true},
		{"not a decimal", ConfigValue{Key: "a", Tier: TierLive, Type: TypeDecimal, Value: "1e3"}, false},
		{"boolean", ConfigValue{Key: "a", Tier: TierLive, Type: TypeBoolean, Value: "true"}, true},
		{"not a boolean", ConfigValue{Key: "a", Tier: TierLive, Type: TypeBoolean, Value: "yes"}, false},
		{"bad key", ConfigValue{Key: "a-b", Tier: TierLive, Type: TypeString}, false},
		{"too long", ConfigValue{Key: "a", Tier: TierLive, Type: TypeString, Value: strings.Repeat("x", 9)}, false},
		{"no type", ConfigValue{Key: "a", Tier: TierLive, Value: "x"}, false},
	}
	for _, tc := range tests {
		err := tc.v.Validate(8)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok = %v", tc.name, err, tc.ok)
		}
	}
}

// Verifies: DAT-091.
// Typed values are put into a body as their JSON types: an integer as a
// number, a boolean as a boolean, a decimal as a string.
func TestInjectedTypes(t *testing.T) {
	t.Parallel()
	mk := func(name string, typ ConfigType, text string) resolvedBinding {
		return resolvedBinding{Binding: Binding{Location: BindJSONPointer, Name: name}, Value: StoredValue{Type: typ, Text: text}}
	}
	got, err := injectBody([]byte(`{"k":1.50}`), []resolvedBinding{
		mk("/n", TypeInteger, "7"), mk("/b", TypeBoolean, "true"), mk("/d", TypeDecimal, "0.015"), mk("/a~1b/c~0d", TypeString, "x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a/b":{"c~d":"x"},"b":true,"d":"0.015","k":1.50,"n":7}`
	if string(got) != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if _, err := injectBody([]byte(`{"a":1,"a":2}`), nil); code(err) != plxerr.InvalidFormat {
		t.Errorf("duplicate members: %v", err)
	}
}
