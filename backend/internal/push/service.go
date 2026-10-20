// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The names of the environment secrets that hold an operator's
// credentials (SEC-106).
const (
	// SecretFCM holds the Google service account key file (JSON).
	SecretFCM = "PUSH_FCM_SERVICE_ACCOUNT"
	// SecretAPNs holds the APNs settings (JSON, see APNsCredentials).
	SecretAPNs = "PUSH_APNS"
)

// Notice says that a channel has a new signed control document.
type Notice struct {
	OrganizationID, AppID, EnvironmentID string
	// Version is the signed version, for the log.
	Version int64
}

// SendJob asks the worker to push to a page of an environment's devices.
type SendJob struct {
	OrganizationID string `json:"organizationId"`
	AppID          string `json:"appId"`
	EnvironmentID  string `json:"environmentId"`
	Version        int64  `json:"version"`
	// After is the last device the previous page ended with; empty for the
	// first page.
	After string `json:"after,omitempty"`
}

// Kind names the job.
func (SendJob) Kind() string { return "push.control" }

// Enqueuer adds a job in the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, job SendJob) error
}

// Secrets opens the operator's credentials.
type Secrets interface {
	OpenEnvironmentSecret(ctx context.Context, organizationID, environmentID, key string) ([]byte, error)
}

// Endpoints override the providers' production endpoints, for tests.
type Endpoints struct {
	FCMSend, FCMToken, APNs, APNsSandbox string
}

// Options configures the service.
type Options struct {
	DB *storage.DB
	// Crypter seals the device tokens (SEC-106).
	Crypter signing.Crypter
	// Secrets opens the operator's credentials; nil leaves every
	// environment without credentials, which sends nothing.
	Secrets Secrets
	// Jobs enqueues the follow-up pages and the first one; nil sends
	// nothing.
	Jobs Enqueuer
	// Enabled turns silent push on. It is off by default (Q22): a server
	// that has not enabled it stores no token and sends no message.
	Enabled bool
	// HTTP is the SSRF-safe client of package httpx; nil uses a default
	// client, which tests replace.
	HTTP *http.Client
	// Endpoints override the production endpoints; their hosts are added
	// to the egress list. Tests only.
	Endpoints Endpoints
	Limits    limits.Set
	Now       func() time.Time
	Log       *slog.Logger
}

// Service stores device push tokens and sends the silent push.
type Service struct {
	o       Options
	client  *http.Client
	senders *senderCache
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Crypter == nil {
		return nil, errors.New("push: a database and a crypter are required")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	e := &o.Endpoints
	for _, d := range []struct {
		dst *string
		def string
	}{{&e.FCMSend, fcmSendEndpoint}, {&e.FCMToken, fcmTokenEndpoint}, {&e.APNs, apnsEndpoint}, {&e.APNsSandbox, apnsSandboxEndpoint}} {
		if *d.dst == "" {
			*d.dst = d.def
		}
	}
	return &Service{
		o: o, client: guardedClient(o.HTTP, e.FCMSend, e.FCMToken, e.APNs, e.APNsSandbox),
		senders: &senderCache{byKey: map[string]Sender{}},
	}, nil
}

// Enabled reports whether silent push is on.
func (s *Service) Enabled() bool { return s.o.Enabled }

// Device names the device a token belongs to.
type Device struct {
	OrganizationID, AppID, EnvironmentID, DeviceID string
}

// patterns of the two providers' tokens, checked before one is stored.
var (
	fcmTokenPattern  = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)
	apnsTokenPattern = regexp.MustCompile(`^[0-9A-Fa-f]+$`)
)

// tokenBinding ties a sealed token to its device.
func tokenBinding(deviceID string) []byte { return []byte("device-push-token:" + deviceID) }

// RegisterToken stores a device's push token, replacing the one it had.
// The token is sealed with envelope encryption and bound to the device. A
// server with silent push off stores nothing and says so (PLX-8033), so a
// runtime does not keep sending tokens nobody uses.
func (s *Service) RegisterToken(ctx context.Context, d Device, provider Provider, token string) error {
	if !s.o.Enabled {
		return plxerr.New(plxerr.PreconditionFailed, "silent push is not enabled on this server")
	}
	if err := s.checkToken(provider, token); err != nil {
		return err
	}
	deviceID, appID, envID, err := parseDevice(d)
	if err != nil {
		return err
	}
	sealed, err := signing.Seal(ctx, s.o.Crypter, []byte(token), tokenBinding(d.DeviceID))
	if err != nil {
		return fmt.Errorf("push.RegisterToken: %w", err)
	}
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		return dbgen.New(tx).UpsertDevicePushToken(ctx, dbgen.UpsertDevicePushTokenParams{
			DeviceID: deviceID, OrganizationID: storage.MustUUID(d.OrganizationID), AppID: appID, EnvironmentID: envID,
			Provider: string(provider), Ciphertext: sealed.Ciphertext, WrappedKey: sealed.WrappedKey, KeyID: sealed.KeyID,
		})
	})
	if err != nil {
		return fmt.Errorf("push.RegisterToken: %w", err)
	}
	return nil
}

// checkToken refuses a provider or token of the wrong form or length.
func (s *Service) checkToken(provider Provider, token string) error {
	max := s.o.Limits.Get(limits.PushTokenBytes)
	if int64(len(token)) > max {
		return plxerr.New(plxerr.LimitExceeded, "a push token may be at most %d bytes (push.tokenBytes)", max)
	}
	ok := false
	switch provider {
	case FCM:
		ok = fcmTokenPattern.MatchString(token)
	case APNs:
		ok = apnsTokenPattern.MatchString(token)
	default:
		return plxerr.New(plxerr.InvalidFormat, "the push provider is fcm or apns")
	}
	if !ok {
		return plxerr.New(plxerr.InvalidFormat, "the push token is not a %s token", provider)
	}
	return nil
}

func parseDevice(d Device) (device, app, env pgtype.UUID, err error) {
	if device, err = storage.UUID(d.DeviceID); err != nil {
		return device, app, env, plxerr.New(plxerr.InvalidFormat, "the device identifier is not valid")
	}
	if app, err = storage.UUID(d.AppID); err != nil {
		return device, app, env, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	if env, err = storage.UUID(d.EnvironmentID); err != nil {
		return device, app, env, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	return device, app, env, nil
}

// ClearToken deletes a device's push token.
func (s *Service) ClearToken(ctx context.Context, d Device) error {
	deviceID, err := storage.UUID(d.DeviceID)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the device identifier is not valid")
	}
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).DeleteDevicePushToken(ctx, deviceID)
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return fmt.Errorf("push.ClearToken: %w", err)
	}
	return nil
}

// NotifyControl queues the pushes for a channel's new control document, in
// the transaction that stored its signature. It does nothing while silent
// push is off.
func (s *Service) NotifyControl(ctx context.Context, tx pgx.Tx, n Notice) error {
	if !s.o.Enabled || s.o.Jobs == nil {
		return nil
	}
	err := s.o.Jobs.Enqueue(ctx, tx, SendJob{OrganizationID: n.OrganizationID, AppID: n.AppID, EnvironmentID: n.EnvironmentID, Version: n.Version})
	if err != nil {
		return fmt.Errorf("push.NotifyControl: %w", err)
	}
	return nil
}

// Send pushes to one page of an environment's devices, after queueing the
// job for the next page, and deletes the tokens the providers report dead.
// A push is an acceleration, not a guarantee: a device that is not reached
// still checks its control document on resume and every five minutes in the
// foreground (SYN-060). A failure to reach a device is therefore logged and
// not retried, which also keeps a retry from queueing the next page twice.
// Only a failure of the job's own database work is returned.
func (s *Service) Send(ctx context.Context, job SendJob) error {
	if !s.o.Enabled {
		return nil
	}
	rows, err := s.page(ctx, job)
	if err != nil || len(rows) == 0 {
		return err
	}
	var dead []pgtype.UUID
	unreached := 0
	for _, row := range rows {
		err := s.sendOne(ctx, job, row)
		switch {
		case err == nil:
		case errors.Is(err, ErrTokenInvalid):
			dead = append(dead, row.DeviceID)
		default:
			unreached++
			s.o.Log.WarnContext(ctx, "a silent push was not delivered", slog.String("environment", job.EnvironmentID), slog.Any("error", err))
		}
	}
	if unreached > 0 {
		s.o.Log.InfoContext(ctx, "silent push finished with devices unreached",
			slog.String("environment", job.EnvironmentID), slog.Int("unreached", unreached), slog.Int("devices", len(rows)))
	}
	return s.deleteTokens(ctx, job.OrganizationID, dead)
}

// page reads the next page of tokens and queues the job for the one after
// it, in one transaction.
func (s *Service) page(ctx context.Context, job SendJob) ([]dbgen.DevicePushToken, error) {
	envID, err := storage.UUID(job.EnvironmentID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	// The first page starts after the nil UUID: a NULL cursor would compare
	// unknown with every device and select none.
	after := pgtype.UUID{Valid: true}
	if job.After != "" {
		if after, err = storage.UUID(job.After); err != nil {
			return nil, plxerr.New(plxerr.InvalidFormat, "the cursor is not valid")
		}
	}
	size := s.o.Limits.Get(limits.PushBatchSize)
	var rows []dbgen.DevicePushToken
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListDevicePushTokens(ctx, dbgen.ListDevicePushTokensParams{EnvironmentID: envID, AfterDevice: after, PageSize: int32(size)}) //nolint:gosec // bounded by the registry
		if err != nil || int64(len(rows)) < size || s.o.Jobs == nil {
			return err //nolint:wrapcheck // one statement
		}
		next := job
		next.After = storage.ID(rows[len(rows)-1].DeviceID)
		return s.o.Jobs.Enqueue(ctx, tx, next) //nolint:wrapcheck // a queue failure
	})
	if err != nil {
		return nil, fmt.Errorf("push.Send: %w", err)
	}
	return rows, nil
}

// sendOne opens a token and sends the message through its provider.
func (s *Service) sendOne(ctx context.Context, job SendJob, row dbgen.DevicePushToken) error {
	sender, ok, err := s.senderFor(ctx, job, Provider(row.Provider))
	if err != nil || !ok {
		return err
	}
	raw, err := signing.Open(ctx, s.o.Crypter, signing.Sealed{Ciphertext: row.Ciphertext, WrappedKey: row.WrappedKey, KeyID: row.KeyID}, tokenBinding(storage.ID(row.DeviceID)))
	if err != nil {
		return fmt.Errorf("%w: open a token: %w", ErrRejected, err)
	}
	return sender.Send(ctx, string(raw), Message{AppID: job.AppID})
}

// deleteTokens removes the tokens the providers no longer know.
func (s *Service) deleteTokens(ctx context.Context, org string, ids []pgtype.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		for _, id := range ids {
			if _, err := q.DeleteDevicePushToken(ctx, id); err != nil {
				return err //nolint:wrapcheck // one statement
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("push.Send: delete dead tokens: %w", err)
	}
	return nil
}

// senderCache keeps a sender per credential so that an access token or
// provider token is reused across messages. The key is a hash of the
// credential, so a rotated secret gets a new sender.
type senderCache struct {
	mu    sync.Mutex
	byKey map[string]Sender
}

// maxSenders bounds the cache; when it is full it starts again.
const maxSenders = 64

// senderFor returns the sender for an environment's credentials of a
// provider, or false when the operator has configured none.
func (s *Service) senderFor(ctx context.Context, job SendJob, provider Provider) (Sender, bool, error) {
	if s.o.Secrets == nil {
		return nil, false, nil
	}
	name := SecretFCM
	if provider == APNs {
		name = SecretAPNs
	}
	raw, err := s.o.Secrets.OpenEnvironmentSecret(ctx, job.OrganizationID, job.EnvironmentID, name)
	if err != nil {
		if code, ok := errors.AsType[*plxerr.Error](err); ok && code.Code == plxerr.ResourceNotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%w: open the %s credentials: %w", ErrRejected, provider, err)
	}
	sum := sha256.Sum256(raw)
	key := string(provider) + ":" + job.EnvironmentID + ":" + hex.EncodeToString(sum[:])
	s.senders.mu.Lock()
	defer s.senders.mu.Unlock()
	if cached, ok := s.senders.byKey[key]; ok {
		return cached, true, nil
	}
	sender, err := s.build(provider, raw)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	if len(s.senders.byKey) >= maxSenders {
		clear(s.senders.byKey)
	}
	s.senders.byKey[key] = sender
	return sender, true, nil
}

// build makes a sender from a provider's credentials.
func (s *Service) build(provider Provider, raw []byte) (Sender, error) {
	e := s.o.Endpoints
	if provider == APNs {
		return newAPNsSender(raw, s.client, e.APNs, e.APNsSandbox, s.o.Now)
	}
	return newFCMSender(raw, s.client, e.FCMSend, e.FCMToken, s.o.Now)
}
