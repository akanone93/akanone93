// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

const (
	// resolveTTL is how long a replica reuses an upstream it read, so a
	// call costs the database one read per upstream per interval. A change
	// reaches every replica within it.
	resolveTTL = 15 * time.Second
	// maxResolved bounds the memory: past it, the cache starts over.
	maxResolved = 4096
)

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// RegistryOptions configures a Registry.
type RegistryOptions struct {
	DB    *storage.DB
	Audit *audit.Log
	IDs   IDs
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Registry stores the upstreams of each app and environment and the
// identity providers of their users (SEC-031, SEC-026). Administrators
// change it through the API; the gateway reads it for every call.
type Registry struct {
	o   RegistryOptions
	now func() time.Time

	mu        sync.Mutex
	resolved  map[string]resolved
	providers map[string]cachedProvider
}

// resolved is one remembered upstream.
type resolved struct {
	upstream Upstream
	until    time.Time
}

// NewRegistry returns the registry.
func NewRegistry(o RegistryOptions) (*Registry, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil {
		return nil, errors.New("gateway: the registry needs a database, an audit log and identifiers")
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Registry{o: o, now: now}, nil
}

// Create registers an upstream and audits it (SEC-031, SEC-140).
func (r *Registry) Create(ctx context.Context, p auth.Principal, u Upstream) (Upstream, error) {
	if err := p.AuthorizeApp(auth.AppManage, u.AppID); err != nil {
		return Upstream{}, fmt.Errorf("gateway: %w", err)
	}
	if err := u.Validate(); err != nil {
		return Upstream{}, err
	}
	id, err := r.o.IDs.New()
	if err != nil {
		return Upstream{}, fmt.Errorf("gateway: %w", err)
	}
	var out Upstream
	err = r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, u.AppID, u.EnvironmentID)
		if err != nil {
			return err
		}
		params, err := insertParams(id, env, u)
		if err != nil {
			return err
		}
		row, err := q.InsertGatewayUpstream(ctx, params)
		if err != nil {
			return failure(err, "upstream")
		}
		out, err = upstreamOf(row)
		if err != nil {
			return err
		}
		return r.record(ctx, tx, p, audit.UpstreamCreated, out)
	})
	return out, err
}

// Update replaces an upstream's settings. Its app and environment do not
// change.
func (r *Registry) Update(ctx context.Context, p auth.Principal, u Upstream) (Upstream, error) {
	if err := u.Validate(); err != nil {
		return Upstream{}, err
	}
	id, err := storage.UUID(u.ID)
	if err != nil {
		return Upstream{}, plxerr.New(plxerr.InvalidFormat, "the upstream identifier is not valid")
	}
	var out Upstream
	var oldName string
	err = r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := q.GetGatewayUpstream(ctx, id)
		if err != nil {
			return failure(err, "upstream")
		}
		if err := p.AuthorizeApp(auth.AppManage, storage.ID(current.AppID)); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		oldName = current.Name
		if (u.AppID != "" && u.AppID != storage.ID(current.AppID)) ||
			(u.EnvironmentID != "" && u.EnvironmentID != storage.ID(current.EnvironmentID)) {
			return plxerr.New(plxerr.InvalidFormat, "an upstream cannot move to another app or environment")
		}
		params, err := updateParams(id, u)
		if err != nil {
			return err
		}
		row, err := q.UpdateGatewayUpstream(ctx, params)
		if err != nil {
			return failure(err, "upstream")
		}
		out, err = upstreamOf(row)
		if err != nil {
			return err
		}
		return r.record(ctx, tx, p, audit.UpstreamUpdated, out)
	})
	if err == nil {
		r.forget(out.EnvironmentID, out.Name)
		r.forget(out.EnvironmentID, oldName)
	}
	return out, err
}

// Delete removes an upstream.
func (r *Registry) Delete(ctx context.Context, p auth.Principal, id string) error {
	uid, err := storage.UUID(id)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the upstream identifier is not valid")
	}
	var gone Upstream
	err = r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetGatewayUpstream(ctx, uid)
		if err != nil {
			return failure(err, "upstream")
		}
		if err := p.AuthorizeApp(auth.AppManage, storage.ID(row.AppID)); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		if gone, err = upstreamOf(row); err != nil {
			return err
		}
		if _, err := q.DeleteGatewayUpstream(ctx, uid); err != nil {
			return failure(err, "upstream")
		}
		return r.record(ctx, tx, p, audit.UpstreamDeleted, gone)
	})
	if err == nil {
		r.forget(gone.EnvironmentID, gone.Name)
	}
	return err
}

// Get returns one upstream.
func (r *Registry) Get(ctx context.Context, p auth.Principal, id string) (Upstream, error) {
	uid, err := storage.UUID(id)
	if err != nil {
		return Upstream{}, plxerr.New(plxerr.InvalidFormat, "the upstream identifier is not valid")
	}
	var out Upstream
	err = r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetGatewayUpstream(ctx, uid)
		if err != nil {
			return failure(err, "upstream")
		}
		if p.AuthorizeApp(auth.AppRead, storage.ID(row.AppID)) != nil {
			return plxerr.New(plxerr.ResourceNotFound, "no such upstream")
		}
		out, err = upstreamOf(row)
		return err
	})
	return out, err
}

// List lists an app's upstreams, optionally of one environment, ordered by
// environment and name. after continues a page: its ID is the environment
// and its Key the name of the last upstream returned.
func (r *Registry) List(ctx context.Context, p auth.Principal, appID, environmentID string, after storage.Cursor, size int32) ([]Upstream, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var env pgtype.UUID
	if environmentID != "" {
		if env, err = storage.UUID(environmentID); err != nil {
			return nil, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
		}
	}
	var out []Upstream
	err = r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListGatewayUpstreams(ctx, dbgen.ListGatewayUpstreamsParams{
			AppID: app, EnvironmentID: env, AfterEnvironment: after.AfterID(), AfterName: after.Key, PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		for _, row := range rows {
			u, err := upstreamOf(row)
			if err != nil {
				return err
			}
			out = append(out, u)
		}
		return nil
	})
	return out, err
}

// Resolve finds the upstream a device of an environment named. The answer
// may be up to resolveTTL old (PLX-6060 when there is none).
func (r *Registry) Resolve(ctx context.Context, organizationID, environmentID, name string) (Upstream, error) {
	key := environmentID + "/" + name
	now := r.now()
	r.mu.Lock()
	hit, ok := r.resolved[key]
	r.mu.Unlock()
	if ok && now.Before(hit.until) {
		return hit.upstream, nil
	}
	env, err := storage.UUID(environmentID)
	if err != nil {
		return Upstream{}, plxerr.New(plxerr.GatewayUpstreamUnknown, "no such upstream")
	}
	var u Upstream
	err = r.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetGatewayUpstreamByName(ctx, dbgen.GetGatewayUpstreamByNameParams{EnvironmentID: env, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.GatewayUpstreamUnknown, "no such upstream")
		}
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		u, err = upstreamOf(row)
		return err
	})
	if err != nil {
		return Upstream{}, err //nolint:wrapcheck // InTx wraps its own failures
	}
	r.mu.Lock()
	if r.resolved == nil || len(r.resolved) >= maxResolved {
		r.resolved = map[string]resolved{}
	}
	r.resolved[key] = resolved{upstream: u, until: now.Add(resolveTTL)}
	r.mu.Unlock()
	return u, nil
}

// forget drops what this replica remembers of an upstream.
func (r *Registry) forget(environmentID, name string) {
	r.mu.Lock()
	delete(r.resolved, environmentID+"/"+name)
	r.mu.Unlock()
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (r *Registry) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return r.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// record audits a change; the detail names the upstream and its host,
// never a credential (SEC-092).
func (r *Registry) record(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, u Upstream) error {
	_, err := r.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action,
		TargetKind: "upstream", TargetID: u.ID,
		Detail: fmt.Sprintf("environment %s, name %s, auth %s, version %d", u.EnvironmentID, u.Name, u.Auth.Kind, u.Version),
	})
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	return nil
}

// environmentOf reads an environment of an app.
func environmentOf(ctx context.Context, q *dbgen.Queries, appID, environmentID string) (dbgen.Environment, error) {
	app, err := storage.UUID(appID)
	if err != nil {
		return dbgen.Environment{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	id, err := storage.UUID(environmentID)
	if err != nil {
		return dbgen.Environment{}, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	env, err := q.GetEnvironment(ctx, id)
	if err != nil {
		return dbgen.Environment{}, failure(err, "environment")
	}
	if env.AppID != app {
		return dbgen.Environment{}, plxerr.New(plxerr.ResourceNotFound, "no such environment in this app")
	}
	return env, nil
}

// failure turns a database error into the refusal a caller understands.
func failure(err error, what string) error {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	case errors.As(err, &pg) && pg.Code == "23505":
		return plxerr.New(plxerr.ResourceExists, "a %s with this name already exists", what)
	}
	return fmt.Errorf("gateway: %s: %w", what, err)
}

// authDoc is the stored form of Auth: references and settings, no secret.
type authDoc struct {
	MTLSSecret   string   `json:"mtlsSecret,omitempty"`
	TokenURL     string   `json:"tokenUrl,omitempty"`
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	Header       string   `json:"header,omitempty"`
	KeySecret    string   `json:"keySecret,omitempty"`
}

// operationDoc is the stored form of Operation.
type operationDoc struct {
	Name        string          `json:"name"`
	Methods     []string        `json:"methods,omitempty"`
	PathPrefix  string          `json:"pathPrefix,omitempty"`
	Assurance   string          `json:"assurance,omitempty"`
	Transaction *transactionDoc `json:"transaction,omitempty"`
	Plugins     []string        `json:"plugins,omitempty"`
}

// transactionDoc is the stored form of Transaction.
type transactionDoc struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Payee    string `json:"payee"`
	Summary  string `json:"summary,omitempty"`
}

// encode builds the columns that hold an upstream's authentication and
// operations.
func encode(u Upstream) (authJSON, operationsJSON []byte, err error) {
	a := u.Auth
	authJSON, err = json.Marshal(authDoc{
		MTLSSecret: a.MTLSSecret, TokenURL: a.TokenURL, ClientID: a.ClientID, ClientSecret: a.ClientSecret,
		Scopes: a.Scopes, Header: a.Header, KeySecret: a.KeySecret,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: encode the authentication: %w", err)
	}
	ops := make([]operationDoc, len(u.Operations))
	for i, op := range u.Operations {
		ops[i] = operationDoc{Name: op.Name, Methods: op.Methods, PathPrefix: op.PathPrefix, Assurance: op.Assurance, Plugins: op.Plugins}
		if t := op.Transaction; t != nil {
			ops[i].Transaction = &transactionDoc{Amount: t.Amount, Currency: t.Currency, Payee: t.Payee, Summary: t.Summary}
		}
	}
	operationsJSON, err = json.Marshal(ops)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: encode the operations: %w", err)
	}
	return authJSON, operationsJSON, nil
}

func insertParams(id string, env dbgen.Environment, u Upstream) (dbgen.InsertGatewayUpstreamParams, error) {
	authJSON, opsJSON, err := encode(u)
	if err != nil {
		return dbgen.InsertGatewayUpstreamParams{}, err
	}
	return dbgen.InsertGatewayUpstreamParams{
		ID: storage.MustUUID(id), OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID,
		Name: u.Name, BaseUrl: u.BaseURL, AuthKind: string(u.Auth.Kind), Auth: authJSON,
		AllowPrivateAddresses: u.AllowPrivate, TimeoutMs: int32(u.Timeout.Milliseconds()), //nolint:gosec // bounded by the limit that checks it
		AllowedMethods: u.Methods, RequiresAssurance: assuranceOrDefault(u.Assurance), Operations: opsJSON, Audience: u.Audience,
	}, nil
}

func updateParams(id pgtype.UUID, u Upstream) (dbgen.UpdateGatewayUpstreamParams, error) {
	authJSON, opsJSON, err := encode(u)
	if err != nil {
		return dbgen.UpdateGatewayUpstreamParams{}, err
	}
	return dbgen.UpdateGatewayUpstreamParams{
		ID: id, Name: u.Name, BaseUrl: u.BaseURL, AuthKind: string(u.Auth.Kind), Auth: authJSON,
		AllowPrivateAddresses: u.AllowPrivate, TimeoutMs: int32(u.Timeout.Milliseconds()), //nolint:gosec // bounded by the limit that checks it
		AllowedMethods: u.Methods, RequiresAssurance: assuranceOrDefault(u.Assurance), Operations: opsJSON, Audience: u.Audience,
	}, nil
}

// assuranceOrDefault names the level a registration that names none gets.
func assuranceOrDefault(level string) string {
	if level == "" {
		return "AL0"
	}
	return level
}

// upstreamOf converts a row.
func upstreamOf(row dbgen.GatewayUpstream) (Upstream, error) {
	var a authDoc
	if err := json.Unmarshal(row.Auth, &a); err != nil {
		return Upstream{}, fmt.Errorf("gateway: the stored authentication does not parse: %w", err)
	}
	var docs []operationDoc
	if err := json.Unmarshal(row.Operations, &docs); err != nil {
		return Upstream{}, fmt.Errorf("gateway: the stored operations do not parse: %w", err)
	}
	ops := make([]Operation, len(docs))
	for i, d := range docs {
		ops[i] = Operation{Name: d.Name, Methods: d.Methods, PathPrefix: d.PathPrefix, Assurance: d.Assurance, Plugins: d.Plugins}
		if t := d.Transaction; t != nil {
			ops[i].Transaction = &Transaction{Amount: t.Amount, Currency: t.Currency, Payee: t.Payee, Summary: t.Summary}
		}
	}
	return Upstream{
		ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID), AppID: storage.ID(row.AppID),
		EnvironmentID: storage.ID(row.EnvironmentID), Name: row.Name, BaseURL: row.BaseUrl,
		Auth: Auth{
			Kind: AuthKind(row.AuthKind), MTLSSecret: a.MTLSSecret, TokenURL: a.TokenURL, ClientID: a.ClientID,
			ClientSecret: a.ClientSecret, Scopes: a.Scopes, Header: a.Header, KeySecret: a.KeySecret,
		},
		AllowPrivate: row.AllowPrivateAddresses, Timeout: time.Duration(row.TimeoutMs) * time.Millisecond,
		Methods: row.AllowedMethods, Assurance: row.RequiresAssurance, Operations: ops, Audience: row.Audience, Version: row.Version,
	}, nil
}
