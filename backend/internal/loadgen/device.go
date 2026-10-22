// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// runtimeVersion and hostBuild are what every synthetic device claims to
// run; they name the load generator, never a real build.
const (
	runtimeVersion = "1.0.0"
	hostBuild      = "plux-load"
)

// rejection is a request the server answered but did not fully accept, such
// as a telemetry batch with refused events.
type rejection struct{ what string }

func (r rejection) Error() string { return r.what }

// device is one synthetic device. Its methods run on one goroutine, except
// nudge, which is safe to call from any.
type device struct {
	id        string
	cfg       *Config
	rec       *recorder
	sess      *session
	pushToken string

	devices   pluxv1connect.DeviceServiceClient
	tokens    pluxv1connect.TokenServiceClient
	manifests pluxv1connect.ManifestServiceClient
	telemetry pluxv1connect.TelemetryServiceClient
	functions pluxv1connect.FunctionServiceClient

	rng            *prng
	manifestETag   string
	controlETag    string
	controlVersion int64
	pending        []*pluxv1.Event
	// release is the sequence of the release the device's manifest names.
	release int64
	// renewAt is when the device renews its access token: three quarters
	// into the token's life, as the runtime renews before it expires;
	// zero when the server gave no expiry.
	renewAt time.Time
	nudges  chan struct{}
}

// newDevice builds the device with the given index; it does not touch the
// network. The index decides its source address and its random stream.
func newDevice(cfg *Config, rec *recorder, index int) (*device, error) {
	address := ""
	if cfg.ForwardedFor {
		address = addressOf(index)
	}
	sess, err := newSession(cfg.BaseURL, address, cfg.Now)
	if err != nil {
		return nil, err
	}
	d := &device{
		cfg: cfg, rec: rec, sess: sess, rng: newPRNG(cfg.Seed, uint64(index)), //nolint:gosec // index is non-negative
		nudges: make(chan struct{}, 1),
	}
	plain := cfg.HTTP
	d.devices = pluxv1connect.NewDeviceServiceClient(plain, cfg.BaseURL, connect.WithInterceptors(sess))
	d.tokens = pluxv1connect.NewTokenServiceClient(plain, cfg.BaseURL, connect.WithInterceptors(sess))
	d.manifests = pluxv1connect.NewManifestServiceClient(plain, cfg.BaseURL, connect.WithInterceptors(sess))
	d.telemetry = pluxv1connect.NewTelemetryServiceClient(plain, cfg.BaseURL, connect.WithInterceptors(sess))
	d.functions = pluxv1connect.NewFunctionServiceClient(plain, cfg.BaseURL, connect.WithInterceptors(sess))
	if cfg.PushTokens {
		d.pushToken = "load_" + randomID()
	}
	return d, nil
}

// addressOf gives device i its own address in 10.0.0.0/8, as the k6 manifest
// test does.
func addressOf(i int) string {
	n := i + 1
	return fmt.Sprintf("10.%d.%d.%d", (n>>16)&255, (n>>8)&255, n&255)
}

// register registers the device with development evidence, takes its first
// access token and, when configured, registers its push token. The whole
// sequence is one observation.
func (d *device) register(ctx context.Context) error {
	start := d.cfg.Now()
	err := d.registerCalls(ctx)
	d.observe(ctx, OpRegister, start, err)
	return err
}

func (d *device) registerCalls(ctx context.Context) error {
	jwk, err := d.sess.publicJWK()
	if err != nil {
		return err
	}
	challenge, err := d.devices.CreateRegistrationChallenge(ctx, connect.NewRequest(&pluxv1.CreateRegistrationChallengeRequest{
		AppId: d.cfg.AppID, Environment: d.cfg.Environment,
	}))
	if err != nil {
		return fmt.Errorf("loadgen.register: %w", err)
	}
	reg, err := d.devices.RegisterAttestedDevice(ctx, connect.NewRequest(&pluxv1.RegisterAttestedDeviceRequest{
		AppId: d.cfg.AppID, Environment: d.cfg.Environment, Platform: "android", RuntimeVersion: runtimeVersion, HostBuild: hostBuild,
		Challenge: challenge.Msg.GetChallenge(), DpopPublicKeyJwk: jwk, KeyStorage: pluxv1.KeyStorage_KEY_STORAGE_SOFTWARE,
		Evidence: &pluxv1.AttestationEvidence{Evidence: &pluxv1.AttestationEvidence_Development{Development: &pluxv1.DevelopmentEvidence{BuildId: hostBuild}}},
	}))
	if err != nil {
		return fmt.Errorf("loadgen.register: %w", err)
	}
	d.id = reg.Msg.GetDevice().GetId()
	if err := d.takeToken(ctx); err != nil {
		return fmt.Errorf("loadgen.register: %w", err)
	}
	if d.pushToken == "" {
		return nil
	}
	_, err = d.devices.RegisterPushToken(ctx, connect.NewRequest(&pluxv1.RegisterPushTokenRequest{
		Provider: pluxv1.PushProvider_PUSH_PROVIDER_FCM, Token: d.pushToken,
	}))
	if err != nil {
		return fmt.Errorf("loadgen.register: %w", err)
	}
	return nil
}

// takeToken obtains a fresh access token and schedules its renewal.
func (d *device) takeToken(ctx context.Context) error {
	now := d.cfg.Now()
	tok, err := d.tokens.RefreshDeviceToken(ctx, connect.NewRequest(&pluxv1.RefreshDeviceTokenRequest{DeviceId: d.id}))
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the callers
	}
	d.sess.setToken(tok.Msg.GetAccessToken())
	d.renewAt = time.Time{}
	if exp := tok.Msg.GetExpiresAt(); exp != nil && exp.AsTime().After(now) {
		d.renewAt = now.Add(exp.AsTime().Sub(now) * 3 / 4)
	}
	return nil
}

// renewToken renews the access token when it is due, so that a device that
// runs longer than a token lives keeps working (SEC-020: tokens last
// minutes).
func (d *device) renewToken(ctx context.Context) {
	if d.renewAt.IsZero() || d.cfg.Now().Before(d.renewAt) {
		return
	}
	start := d.cfg.Now()
	err := d.takeToken(ctx)
	if err != nil {
		err = fmt.Errorf("loadgen.renewToken: %w", err)
	}
	d.observe(ctx, OpToken, start, err)
}

// observe records a request unless it failed because the run ended: a
// request cut off by the end of the run says nothing about the server.
func (d *device) observe(ctx context.Context, name string, start time.Time, err error) {
	if err != nil && ctx.Err() != nil {
		return
	}
	d.rec.observe(name, d.cfg.Now().Sub(start), err)
}

// nudge asks the device to check its control document at once, as the host
// app does when a silent push arrives (SYN-060). A nudge that finds one
// already waiting is dropped: the check it asks for is already due.
func (d *device) nudge() {
	select {
	case d.nudges <- struct{}{}:
	default:
	}
}

// checkManifest asks whether the manifest changed, with the ETag it holds.
func (d *device) checkManifest(ctx context.Context) {
	start := d.cfg.Now()
	res, err := d.manifests.GetManifest(ctx, connect.NewRequest(&pluxv1.GetManifestRequest{
		AppId: d.cfg.AppID, Environment: d.cfg.Environment, Channel: d.cfg.Channel, IfNoneMatch: d.manifestETag,
	}))
	d.observe(ctx, OpManifest, start, err)
	if err == nil && res.Msg.GetEtag() != "" {
		d.manifestETag = res.Msg.GetEtag()
	}
	if err == nil && res.Msg.GetManifest() != nil {
		d.release = res.Msg.GetManifest().GetReleaseSequence()
	}
}

// checkControl fetches the signed control document with its ETag and, when
// its version is newer than the device's, reports control_applied. A
// channel without a signed document yet is not an error: nothing is
// switched.
func (d *device) checkControl(ctx context.Context) {
	start := d.cfg.Now()
	res, err := d.manifests.GetSignedControl(ctx, connect.NewRequest(&pluxv1.GetSignedControlRequest{
		Channel: d.cfg.Channel, IfNoneMatch: d.controlETag,
	}))
	seen := d.cfg.Now()
	if connect.CodeOf(err) == connect.CodeNotFound {
		err = nil
	}
	d.observe(ctx, OpControl, start, err)
	if err != nil || res == nil || res.Msg.GetNotModified() {
		return
	}
	d.controlETag = res.Msg.GetEtag()
	if v := res.Msg.GetVersion(); v > d.controlVersion {
		d.controlVersion = v
		d.applied(ctx, v, seen)
	}
}

// applied reports that the device applied a control document. Operational
// events are sent at once, not with the next batch (necessary category,
// SEC-161), which is what makes the propagation time one the server can
// measure.
func (d *device) applied(ctx context.Context, version int64, at time.Time) {
	d.queue("control_applied", "", fmt.Sprintf(`{"version":%d,"switches":0,"switches_changed":0,"outcome":"applied"}`, version))
	d.rec.markApplied()
	if d.cfg.OnApplied != nil {
		d.cfg.OnApplied(Applied{DeviceID: d.id, Version: version, At: at})
	}
	d.flush(ctx)
}

// queue buffers an event for the next batch, attributed to the release the
// device runs, as the runtime attributes its events: a rollout's health gate
// compares the releases by these counts (REL-053).
func (d *device) queue(name, route, fields string) {
	d.pending = append(d.pending, &pluxv1.Event{
		Name: name, Time: timestamppb.New(d.cfg.Now()), DeviceId: d.id, ReleaseSequence: d.release, Route: route, Fields: []byte(fields),
	})
}

// sessionStart queues the session_start every session begins with.
func (d *device) sessionStart() {
	d.queue("session_start", "", fmt.Sprintf(
		`{"runtime_version":%q,"host_build":%q,"os_version":"synthetic","device_class":"phone","locale":"en"}`, runtimeVersion, hostBuild))
}

// activity queues the events of a minute of use: two screens and an action,
// and an error when the run injects a regression.
func (d *device) activity() {
	d.queue("screen_view", "/home", `{"screen":"home"}`)
	d.queue("screen_view", "/detail", `{"screen":"detail"}`)
	d.queue("action_run", "/detail", `{"action":"refresh","steps":3}`)
	if d.cfg.InjectErrors && (d.cfg.ErrorRelease == 0 || d.release == d.cfg.ErrorRelease) {
		d.queue("error", "/detail", `{"code":"synthetic_regression"}`)
	}
}

// flush sends the buffered events as one batch. The consent it declares
// covers the analytics events it carries, nothing more.
func (d *device) flush(ctx context.Context) {
	if len(d.pending) == 0 {
		return
	}
	events := d.pending
	d.pending = nil
	start := d.cfg.Now()
	res, err := d.telemetry.IngestEvents(ctx, connect.NewRequest(&pluxv1.IngestEventsRequest{
		AppId: d.cfg.AppID, Environment: d.cfg.Environment, Events: events,
		Consent: &pluxv1.Consent{Analytics: true},
	}))
	if err == nil && res.Msg.GetRejected() > 0 {
		err = rejection{what: fmt.Sprintf("%d events rejected", res.Msg.GetRejected())}
	}
	d.observe(ctx, OpTelemetry, start, err)
}

// callFunction invokes the configured function with a fresh idempotency key.
func (d *device) callFunction(ctx context.Context) {
	f := d.cfg.Function
	start := d.cfg.Now()
	_, err := d.functions.Invoke(ctx, connect.NewRequest(&pluxv1.FunctionServiceInvokeRequest{
		Function: f.Name, Version: f.Version, Input: f.Input, IdempotencyKey: randomID(),
	}))
	d.observe(ctx, OpFunction, start, err)
}
