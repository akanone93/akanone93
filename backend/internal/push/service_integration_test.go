// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package push_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/push"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// jobs records the jobs the service enqueues.
type jobs struct {
	mu   sync.Mutex
	list []push.SendJob
}

func (j *jobs) Enqueue(_ context.Context, _ pgx.Tx, job push.SendJob) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.list = append(j.list, job)
	return nil
}

func (j *jobs) take() []push.SendJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := j.list
	j.list = nil
	return out
}

// secrets is a secret store with the credentials of one environment.
type secrets map[string][]byte

func (s secrets) OpenEnvironmentSecret(_ context.Context, _, _, key string) ([]byte, error) {
	if v, ok := s[key]; ok {
		return v, nil
	}
	return nil, plxerr.New(plxerr.ResourceNotFound, "no such secret")
}

// world is an organisation with an app, an environment and devices, and a
// fake FCM.
type world struct {
	db      *storage.DB
	crypter *signing.File
	org     string
	app     string
	env     string
	ids     *uuid7.Generator

	mu    sync.Mutex
	sent  []map[string]any
	codes map[string]int
	srv   *httptest.Server
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{db: storagetest.Open(t), ids: uuid7.NewGenerator(time.Now, rand.Reader), codes: map[string]int{}}
	var err error
	if w.crypter, err = signing.NewFile(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	w.org, w.app, w.env = w.id(t), w.id(t), w.id(t)
	ctx := context.Background()
	if _, err := w.db.Pool().Exec(ctx, `INSERT INTO organizations (id, key, name) VALUES ($1, 'acme', 'Acme')`, storage.MustUUID(w.org)); err != nil {
		t.Fatal(err)
	}
	w.inTx(t, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'demo', 'Demo')`, storage.MustUUID(w.app), storage.MustUUID(w.org)); err != nil {
			return err //nolint:wrapcheck // a test
		}
		_, err := tx.Exec(ctx, `INSERT INTO environments (id, organization_id, app_id, key, name, production) VALUES ($1, $2, $3, 'production', 'Production', true)`,
			storage.MustUUID(w.env), storage.MustUUID(w.org), storage.MustUUID(w.app))
		return err //nolint:wrapcheck // a test
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(rw http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(rw).Encode(map[string]any{"access_token": "access", "expires_in": 3600})
	})
	mux.HandleFunc("/v1/projects/demo-project/messages:send", func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var msg struct {
			Message map[string]any `json:"message"`
		}
		_ = json.Unmarshal(body, &msg)
		w.mu.Lock()
		w.sent = append(w.sent, msg.Message)
		code := w.codes[fmt.Sprint(msg.Message["token"])]
		w.mu.Unlock()
		if code != 0 {
			rw.WriteHeader(code)
		}
	})
	w.srv = httptest.NewServer(mux)
	t.Cleanup(w.srv.Close)
	return w
}

func (w *world) id(t *testing.T) string {
	t.Helper()
	u, err := w.ids.New()
	if err != nil {
		t.Fatal(err)
	}
	return u.String()
}

func (w *world) inTx(t *testing.T, f func(context.Context, pgx.Tx) error) {
	t.Helper()
	if err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: w.org}, f); err != nil {
		t.Fatal(err)
	}
}

// device registers a device row and returns its identity.
func (w *world) device(t *testing.T) push.Device {
	t.Helper()
	id := w.id(t)
	w.inTx(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO devices (id, organization_id, app_id, environment_id, platform, secret_hash) VALUES ($1, $2, $3, $4, 'android', '\x00')`,
			storage.MustUUID(id), storage.MustUUID(w.org), storage.MustUUID(w.app), storage.MustUUID(w.env))
		return err //nolint:wrapcheck // a test
	})
	return push.Device{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.env, DeviceID: id}
}

func (*world) credentials(t *testing.T) secrets {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	acct, err := json.Marshal(push.ServiceAccount{
		ProjectID: "demo-project", ClientEmail: "push@example.iam", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return secrets{push.SecretFCM: acct}
}

func (w *world) service(t *testing.T, enabled bool, q *jobs, s secrets, set limits.Set) *push.Service {
	t.Helper()
	svc, err := push.New(push.Options{
		DB: w.db, Crypter: w.crypter, Secrets: s, Jobs: q, Enabled: enabled, HTTP: w.srv.Client(), Limits: set,
		Endpoints: push.Endpoints{FCMSend: w.srv.URL, FCMToken: w.srv.URL + "/token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func (w *world) count(t *testing.T) int {
	t.Helper()
	var n int
	w.inTx(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM device_push_tokens`).Scan(&n)
	})
	return n
}

// Verifies: SYN-060, Q22.
// Silent push is off unless turned on: no token is stored, no job is
// queued and nothing is sent.
func TestPushIsOffByDefault_SYN_060(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	q := &jobs{}
	svc := w.service(t, false, q, w.credentials(t), limits.Set{})
	d := w.device(t)
	err := svc.RegisterToken(context.Background(), d, push.FCM, "token-1")
	if e, ok := err.(*plxerr.Error); !ok || e.Code != plxerr.PreconditionFailed { //nolint:errorlint // a test
		t.Fatalf("RegisterToken = %v", err)
	}
	if svc.Enabled() || w.count(t) != 0 {
		t.Fatalf("a token was stored while push is off")
	}
	if err := svc.NotifyControl(context.Background(), nil, push.Notice{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.env}); err != nil || len(q.take()) != 0 {
		t.Errorf("NotifyControl queued a job while push is off: %v", err)
	}
	if err := svc.Send(context.Background(), push.SendJob{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.env}); err != nil || len(w.sent) != 0 {
		t.Errorf("Send sent while push is off: %v", err)
	}
}

// Verifies: SYN-060, SEC-106.
// A push token is stored sealed, bound to its device, replaced when
// registered again, checked for form and size, and deleted with its device.
func TestPushTokenIsStoredEncrypted_SYN_060(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc := w.service(t, true, &jobs{}, nil, limits.Set{})
	ctx := context.Background()
	d := w.device(t)
	const token = "fcm-token_ABC:123-secret-value"
	if err := svc.RegisterToken(ctx, d, push.FCM, token); err != nil {
		t.Fatalf("RegisterToken: %v", err)
	}
	var cipher []byte
	var provider string
	w.inTx(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT ciphertext, provider FROM device_push_tokens`).Scan(&cipher, &provider)
	})
	if bytes.Contains(cipher, []byte("secret-value")) || provider != "fcm" {
		t.Fatalf("the token is stored in the clear or for the wrong provider (%s)", provider)
	}
	if err := svc.RegisterToken(ctx, d, push.APNs, "00ab"); err != nil || w.count(t) != 1 {
		t.Fatalf("registering again must replace: %v %d", err, w.count(t))
	}
	for name, c := range map[string]struct {
		p push.Provider
		t string
		c plxerr.Code
	}{
		"unknown provider": {"sms", "abc", plxerr.InvalidFormat},
		"fcm with space":   {push.FCM, "a b", plxerr.InvalidFormat},
		"apns not hex":     {push.APNs, "xyz", plxerr.InvalidFormat},
		"too long":         {push.FCM, strings.Repeat("a", 1025), plxerr.LimitExceeded},
		"empty":            {push.FCM, "", plxerr.InvalidFormat},
	} {
		if err := svc.RegisterToken(ctx, d, c.p, c.t); err == nil || err.(*plxerr.Error).Code != c.c { //nolint:errorlint // a test
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := svc.ClearToken(ctx, d); err != nil || w.count(t) != 0 {
		t.Fatalf("ClearToken: %v", err)
	}
	if err := svc.RegisterToken(ctx, d, push.FCM, token); err != nil {
		t.Fatal(err)
	}
	w.inTx(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM devices WHERE id = $1`, storage.MustUUID(d.DeviceID))
		return err //nolint:wrapcheck // a test
	})
	if w.count(t) != 0 {
		t.Errorf("the token outlived its device")
	}
}

// Verifies: SYN-060.
// A control change queues a push job; the job sends the marker to every
// registered device in pages, deletes a token the provider reports dead and
// skips an environment without credentials.
func TestPushSendsInPagesAndDropsDeadTokens_SYN_060(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	q := &jobs{}
	set, err := limits.Defaults().Tighten(limits.PushBatchSize, limits.ScopeInstallation, 2)
	if err != nil {
		t.Fatal(err)
	}
	svc := w.service(t, true, q, w.credentials(t), set)
	ctx := context.Background()
	tokens := map[string]bool{}
	for i := range 5 {
		d := w.device(t)
		token := fmt.Sprintf("token-%d", i)
		tokens[token] = true
		if err := svc.RegisterToken(ctx, d, push.FCM, token); err != nil {
			t.Fatal(err)
		}
	}
	w.codes["token-3"] = http.StatusNotFound
	if err := svc.NotifyControl(ctx, nil, push.Notice{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.env, Version: 4}); err != nil {
		t.Fatal(err)
	}
	pages := 0
	for pending := q.take(); len(pending) > 0; pending = q.take() {
		for _, job := range pending {
			pages++
			if err := svc.Send(ctx, job); err != nil {
				t.Fatalf("Send: %v", err)
			}
		}
	}
	if pages != 3 {
		t.Errorf("%d pages for five devices at two a page, want 3", pages)
	}
	if len(w.sent) != 5 {
		t.Fatalf("%d messages, want 5", len(w.sent))
	}
	for _, m := range w.sent {
		data, _ := m["data"].(map[string]any)
		if _, shows := m["notification"]; shows || data["plux"] != "control" || data["app"] != w.app || len(data) != 2 {
			t.Errorf("message = %v", m)
		}
		delete(tokens, fmt.Sprint(m["token"]))
	}
	if len(tokens) != 0 {
		t.Errorf("devices never reached: %v", tokens)
	}
	if w.count(t) != 4 {
		t.Errorf("%d tokens left, want the dead one deleted", w.count(t))
	}

	// An environment whose operator has configured nothing is skipped.
	bare := w.service(t, true, q, secrets{}, set)
	before := len(w.sent)
	if err := bare.Send(ctx, push.SendJob{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.env}); err != nil || len(w.sent) != before {
		t.Errorf("without credentials: %v, %d sent", err, len(w.sent)-before)
	}
}
