// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gitsync

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// DefaultUsername is sent with the token when no user name is set; it is
// what the large Git hosts accept for an access token.
const DefaultUsername = "x-access-token"

// Settings is the Git export of one app (GOV-011).
type Settings struct {
	AppID   string
	Enabled bool
	// RemoteURL is the HTTPS address of the repository.
	RemoteURL string
	Branch    string
	// Username is sent with the credential; empty means DefaultUsername.
	Username string
	// CredentialEnvironmentID and CredentialKey name the environment
	// secret that holds the token.
	CredentialEnvironmentID string
	CredentialKey           string
	UpdatedAt               time.Time
}

func settingsOf(r dbgen.GitSetting) Settings {
	return Settings{
		AppID: storage.ID(r.AppID), Enabled: r.Enabled, RemoteURL: r.RemoteUrl, Branch: r.Branch, Username: r.Username,
		CredentialEnvironmentID: storage.ID(r.CredentialEnvironmentID), CredentialKey: r.CredentialKey,
		UpdatedAt: storage.Time(r.UpdatedAt),
	}
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// Get returns the export settings of an app.
func (s *Service) Get(ctx context.Context, p auth.Principal, appID string) (Settings, error) {
	if err := p.AuthorizeApp(auth.GitExport, appID); err != nil {
		return Settings{}, fmt.Errorf("gitsync: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return Settings{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var out Settings
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetGitSettings(ctx, app)
		if err != nil {
			return missing(err, "Git export setting")
		}
		out = settingsOf(row)
		return nil
	})
	return out, err
}

// Configure sets the export of an app and audits the change (GOV-011).
func (s *Service) Configure(ctx context.Context, p auth.Principal, in Settings) (Settings, error) {
	if err := p.AuthorizeApp(auth.GitExport, in.AppID); err != nil {
		return Settings{}, fmt.Errorf("gitsync: %w", err)
	}
	app, err := storage.UUID(in.AppID)
	if err != nil {
		return Settings{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	in, err = s.validate(ctx, in)
	if err != nil {
		return Settings{}, err
	}
	var out Settings
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetApp(ctx, app); err != nil {
			return missing(err, "app")
		}
		env, err := credentialEnvironment(ctx, q, in, app)
		if err != nil {
			return err
		}
		row, err := q.UpsertGitSettings(ctx, dbgen.UpsertGitSettingsParams{
			AppID: app, OrganizationID: storage.MustUUID(p.OrganizationID), Enabled: in.Enabled, RemoteUrl: in.RemoteURL,
			Branch: in.Branch, Username: in.Username, CredentialEnvironmentID: env, CredentialKey: in.CredentialKey,
			UpdatedBy: userID(p),
		})
		if err != nil {
			return fmt.Errorf("gitsync: save the setting: %w", err)
		}
		out = settingsOf(row)
		return s.record(ctx, tx, p.Actor(), p.OrganizationID, audit.GitExportConfigured, "app", in.AppID, fmt.Sprintf("enabled=%t", in.Enabled))
	})
	return out, err
}

// credentialEnvironment checks that the environment of the credential
// belongs to the app, and returns it; NULL where none is named.
func credentialEnvironment(ctx context.Context, q *dbgen.Queries, in Settings, app pgtype.UUID) (pgtype.UUID, error) {
	if in.CredentialEnvironmentID == "" {
		return pgtype.UUID{}, nil
	}
	id := storage.MustUUID(in.CredentialEnvironmentID)
	env, err := q.GetEnvironment(ctx, id)
	if err != nil || env.AppID != app {
		return pgtype.UUID{}, plxerr.New(plxerr.ResourceNotFound, "the credential's environment is not an environment of the app")
	}
	return id, nil
}

// validate checks and completes a setting.
func (s *Service) validate(ctx context.Context, in Settings) (Settings, error) {
	if in.Branch == "" {
		in.Branch = "main"
	}
	if err := plumbing.NewBranchReferenceName(in.Branch).Validate(); err != nil {
		return in, plxerr.New(plxerr.InvalidFormat, "%q is not a branch name", in.Branch)
	}
	host, err := remoteHost(in.RemoteURL)
	if err != nil {
		return in, err
	}
	if _, err := allowedAddresses(ctx, s.o.Policy, s.o.Resolver, host); err != nil && isRefusal(err) {
		return in, plxerr.Wrap(plxerr.GitRemoteRefused, err, "the remote's address is not one the server may connect to")
	}
	for _, r := range in.Username {
		if unicode.IsControl(r) || r == ':' {
			return in, plxerr.New(plxerr.InvalidFormat, "the user name must not hold control characters or a colon")
		}
	}
	if in.Enabled && (in.CredentialEnvironmentID == "" || in.CredentialKey == "") {
		return in, plxerr.New(plxerr.MissingProperty, "an enabled export needs the environment and the key of the secret that holds the credential")
	}
	if in.CredentialEnvironmentID != "" {
		if _, err := storage.UUID(in.CredentialEnvironmentID); err != nil {
			return in, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
		}
	}
	return in, nil
}

// isRefusal reports whether an address check ended in a refusal, as
// against a name that would not resolve now, which configuring tolerates.
func isRefusal(err error) bool { return errors.Is(err, errRefusedAddress) }

// remoteHost checks the form of a remote address and returns its host:
// HTTPS, a host, no credentials in the address and nothing after the path.
func remoteHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", plxerr.New(plxerr.InvalidFormat, "the remote must be an https:// address")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", plxerr.New(plxerr.InvalidFormat, "the remote must not hold credentials, a query or a fragment; the credential is a secret of an environment")
	}
	return u.Hostname(), nil
}

// missing maps no rows to not found.
func missing(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("gitsync: %s: %w", what, err)
}
