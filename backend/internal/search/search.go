// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// The kinds of result (SRV-061).
const (
	KindApp            = "app"
	KindPlugin         = "plugin"
	KindPage           = "page"
	KindComponent      = "component"
	KindTemplate       = "template"
	KindTranslationKey = "translation_key"
	KindFunction       = "function"
)

// kinds lists every kind, in the order the API documents them.
var kinds = []string{KindApp, KindPlugin, KindPage, KindComponent, KindTemplate, KindTranslationKey, KindFunction}

// Kinds returns the kinds a search can be restricted to.
func Kinds() []string { return slices.Clone(kinds) }

// Options are what the service needs.
type Options struct {
	DB *storage.DB
	// Tenancy supplies the limits in force for the organisation.
	Tenancy *tenancy.Service
}

// Service searches the organisation's apps for a caller.
type Service struct{ o Options }

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.Tenancy == nil {
		return nil, errors.New("search: a database and tenancy are required")
	}
	return &Service{o: o}, nil
}

// Query is one search.
type Query struct {
	// Text is what the caller typed: words, each matched as a prefix.
	Text string
	// Kinds restricts the kinds of result; empty means all.
	Kinds []string
	// PageSize is the number of results wanted; zero or more than
	// search.resultsPerPage gives search.resultsPerPage.
	PageSize int32
	// Offset is the number of results to skip.
	Offset int32
}

// Hit is one result.
type Hit struct {
	// Kind is one of the Kind constants.
	Kind string
	// ID is the identifier of the app, plugin or document.
	ID string
	// AppID is the app the result belongs to.
	AppID string
	// Name is the result's name: an app or plugin name, or a document's key.
	Name string
	// Path is the app or plugin key path, or the document's path in the
	// project layout.
	Path string
	// Snippet is the matching text with each hit between « and ».
	Snippet string
	// Rank orders results; higher is a better match.
	Rank float32
}

// Result is a page of hits with the page size that applied.
type Result struct {
	Hits []Hit
	// PageSize is the size of the page that was asked for, after the
	// limit; a page with fewer hits is the last.
	PageSize int32
}

// Search returns the hits of a query that the principal may see, best
// first (SRV-061).
func (s *Service) Search(ctx context.Context, p auth.Principal, q Query) (Result, error) {
	allowed, err := searchableApps(p)
	if err != nil {
		return Result{}, err
	}
	tsq, err := tsquery(q.Text)
	if err != nil {
		return Result{}, err
	}
	if err := checkKinds(q.Kinds); err != nil {
		return Result{}, err
	}
	if q.Offset < 0 {
		return Result{}, plxerr.New(plxerr.OutOfRange, "the offset must not be negative")
	}
	var out Result
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		lim, err := s.o.Tenancy.Effective(ctx, tx, p.OrganizationID, "", "")
		if err != nil {
			return err //nolint:wrapcheck // databaseFailure wraps it
		}
		out.PageSize = pageSize(q.PageSize, lim.Get(limits.SearchResultsPerPage))
		rows, err := dbgen.New(tx).Search(ctx, dbgen.SearchParams{
			Query: tsq, Allowed: allowed, Excluded: excludedApps(p), Kinds: nonNil(q.Kinds), PageSize: out.PageSize, PageOffset: q.Offset,
		})
		if err != nil {
			return err //nolint:wrapcheck // databaseFailure wraps it
		}
		for _, r := range rows {
			out.Hits = append(out.Hits, Hit{
				Kind: r.Kind, ID: storage.ID(r.Ref), AppID: storage.ID(r.AppID), Name: r.Name,
				Path: r.Path, Snippet: r.Snippet, Rank: r.Rank,
			})
		}
		return nil
	})
	if err != nil {
		return Result{}, databaseFailure(err)
	}
	return out, nil
}

// searchableApps returns the apps the principal may search: nil for all
// of the organisation's, which is how the statement reads "no restriction",
// or the list of apps it holds search.read on. A principal that holds the
// permission nowhere is refused.
func searchableApps(p auth.Principal) ([]pgtype.UUID, error) {
	if p.OrganizationID == "" {
		return nil, authorizeSearch(p)
	}
	if p.Holds(auth.SearchRead) {
		return nil, nil
	}
	allowed := []pgtype.UUID{}
	for _, app := range slices.Sorted(appsOf(p)) {
		if !slices.Contains(p.AppPermissions[app], auth.SearchRead) {
			continue
		}
		id, err := storage.UUID(app)
		if err != nil {
			continue // a malformed key can match no row
		}
		allowed = append(allowed, id)
	}
	if len(allowed) == 0 {
		return nil, authorizeSearch(p)
	}
	return allowed, nil
}

// authorizeSearch refuses a principal that does not hold search.read.
func authorizeSearch(p auth.Principal) error {
	if err := p.Authorize(auth.SearchRead); err != nil {
		return fmt.Errorf("search: %w", err)
	}
	return nil
}

// excludedApps returns the apps a restriction takes search.read away from,
// for the statement to leave out even when the permission is held for the
// whole organisation (GOV-006). It is never nil, so that the driver sends
// an empty array.
func excludedApps(p auth.Principal) []pgtype.UUID {
	out := []pgtype.UUID{}
	for _, app := range p.ExcludedApps(auth.SearchRead) {
		if id, err := storage.UUID(app); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// appsOf iterates over the apps a principal holds permissions on.
func appsOf(p auth.Principal) func(func(string) bool) {
	return func(yield func(string) bool) {
		for app := range p.AppPermissions {
			if !yield(app) {
				return
			}
		}
	}
}

// pageSize applies the limit to the size a caller asked for.
func pageSize(asked int32, limit int64) int32 {
	most := int32(min(limit, 1<<30)) //nolint:gosec // bounded by the min
	if asked <= 0 || asked > most {
		return most
	}
	return asked
}

// nonNil returns a slice the driver encodes as an empty array, not NULL.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// checkKinds refuses a kind the search does not know.
func checkKinds(want []string) error {
	for _, k := range want {
		if !slices.Contains(kinds, k) {
			return plxerr.New(plxerr.InvalidEnumValue, "%q is not a kind of search result; use one of %s", k, strings.Join(kinds, ", "))
		}
	}
	return nil
}

// tsquery turns the caller's words into a tsquery in which every word is a
// prefix term and all must match. Only letters, digits and marks reach the
// result; everything else separates words, so the text cannot carry
// tsquery syntax.
func tsquery(text string) (string, error) {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	if len(words) == 0 {
		return "", plxerr.New(plxerr.MissingProperty, "a search needs at least one word")
	}
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = w + ":*"
	}
	return strings.Join(terms, " & "), nil
}

// databaseFailure maps the one database refusal a caller can cause, a query
// with more terms than a tsquery holds, and wraps the rest.
func databaseFailure(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "54000" {
		return plxerr.New(plxerr.OutOfRange, "the search has too many words")
	}
	var perr *plxerr.Error
	if errors.As(err, &perr) {
		return err
	}
	return fmt.Errorf("search: %w", err)
}
