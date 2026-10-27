// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package library stores the templates and components authored in Studio:
// versions that never change, visibility with a scan before anything is
// public, library files, and the version an app adopts (STU-080 to STU-086,
// ADR-0098). Authorisation is decided here, from the principal, and every
// query runs in the principal's organisation under row-level security.
package library

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Kind is what an item is.
type Kind string

// The two kinds of item.
const (
	KindTemplate  Kind = "template"
	KindComponent Kind = "component"
)

// The visibility levels of an item (STU-081).
const (
	Private      = "private"
	Organization = "organization"
	Public       = "public"
)

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// Instance is one use of a component in an app's drafts, with the names of
// the props and slots it overrides (STU-084).
type Instance struct {
	File, Path string
	Props      []string
	Slots      []string
}

// Instances finds the uses of a component in an app. The document service
// provides it from the app's drafts.
type Instances interface {
	ComponentInstances(ctx context.Context, p auth.Principal, appID, componentID string) ([]Instance, error)
}

// Options are what the service needs. Instances may be nil, and an adoption
// then reports no conflicts.
type Options struct {
	DB        *storage.DB
	Audit     *audit.Log
	IDs       IDs
	Instances Instances
	// Limits returns the limits in force for an organisation (LIM-001); nil
	// means the defaults.
	Limits func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)
}

// Service stores the library.
type Service struct {
	o         Options
	validator *schema.Validator
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	switch {
	case o.DB == nil:
		return nil, errors.New("library: a database is required")
	case o.Audit == nil:
		return nil, errors.New("library: an audit log is required")
	case o.IDs == nil:
		return nil, errors.New("library: an identifier generator is required")
	}
	v, err := schema.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("library: %w", err)
	}
	return &Service{o: o, validator: v}, nil
}

// Item is a template or component.
type Item struct {
	ID, OrganizationID       string
	Kind                     Kind
	Key, DocumentID, Name    string
	Description              string
	TemplateKind             string
	Visibility               string
	HeadVersion              int32
	CreatedBy, CreatedByName string
	CreatedAt, UpdatedAt     time.Time
}

// Version is one immutable version of an item.
type Version struct {
	ItemID        string
	Number        int32
	Content       []byte
	ContentSHA256 []byte
	Note          string
	CreatedByName string
	CreatedAt     time.Time
}

// Draft is what an item or a version is made from.
type Draft struct {
	Key, Name, Description, TemplateKind string
	// Content is the template or component document, as JSON.
	Content []byte
	Note    string
}

// Adoption is the version of a component an app adopted.
type Adoption struct {
	AppID, ItemID string
	Version       int32
	AdoptedBy     string
	AdoptedAt     time.Time
}

// Conflict is an instance whose overrides the adopted version no longer fits.
type Conflict struct {
	File, Path   string
	RemovedProps []string
	RemovedSlots []string
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

func (s *Service) limitsFor(ctx context.Context, tx pgx.Tx, p auth.Principal) (limits.Set, error) {
	if s.o.Limits == nil {
		return limits.Defaults(), nil
	}
	return s.o.Limits(ctx, tx, p.OrganizationID) //nolint:wrapcheck // a domain error
}

func authorize(p auth.Principal, want auth.Permission) error {
	if err := p.Authorize(want); err != nil {
		return fmt.Errorf("library: %w", err)
	}
	return nil
}

func failure(err error, what string) error {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	case errors.As(err, &pg) && pg.Code == "23505":
		return plxerr.New(plxerr.ResourceExists, "the organisation already has a %s with this key or document identifier", what)
	default:
		return fmt.Errorf("library: %s: %w", what, err)
	}
}

func parseID(s, what string) (pgtype.UUID, error) {
	id, err := storage.UUID(s)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the %s identifier is not valid", what)
	}
	return id, nil
}

func itemOf(r dbgen.LibraryItem) Item {
	return Item{
		ID: storage.ID(r.ID), OrganizationID: storage.ID(r.OrganizationID), Kind: Kind(r.Kind), Key: r.Key, DocumentID: r.DocumentID,
		Name: r.Name, Description: r.Description, TemplateKind: r.TemplateKind, Visibility: r.Visibility, HeadVersion: r.HeadVersion,
		CreatedBy: r.CreatedBy, CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func versionOf(r dbgen.LibraryVersion) Version {
	return Version{
		ItemID: storage.ID(r.ItemID), Number: r.Version, Content: r.Content, ContentSHA256: r.ContentSha256,
		Note: r.Note, CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt.Time,
	}
}

// load reads an item of the right kind that the principal may see.
func load(ctx context.Context, q *dbgen.Queries, p auth.Principal, kind Kind, id string) (Item, error) {
	uid, err := parseID(id, string(kind))
	if err != nil {
		return Item{}, err
	}
	row, err := q.GetLibraryItem(ctx, dbgen.GetLibraryItemParams{ID: uid, Actor: p.UserID})
	if err != nil {
		return Item{}, failure(err, string(kind))
	}
	it := itemOf(row)
	if it.Kind != kind {
		return Item{}, plxerr.New(plxerr.ResourceNotFound, "no such %s", kind)
	}
	return it, nil
}

// mayEdit decides whether the principal may change an item of their
// organisation: the author with plugin.edit, anyone else with library.manage
// (ADR-0098). An item of another organisation is never editable; public ones
// are visible to everyone and still belong to their organisation.
func mayEdit(p auth.Principal, it Item) error {
	if it.OrganizationID != p.OrganizationID {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", it.Kind)
	}
	if it.CreatedBy == p.UserID {
		return authorize(p, auth.PluginEdit)
	}
	return authorize(p, auth.LibraryManage)
}

// document is a parsed, validated template or component document.
type document struct {
	tree     map[string]any
	canon    []byte
	id, key  string
	name     string
	findings []Finding
}

// parseDocument validates content as the document of a kind: strict JSON,
// within the registry's size and depth, valid against its schema, and in
// canonical form (RFC 8785), so the same document always has the same hash.
func (s *Service) parseDocument(kind Kind, content []byte, lim limits.Set) (document, error) {
	if int64(len(content)) > lim.Get(limits.DocumentFileSize) {
		return document{}, plxerr.New(plxerr.LimitExceeded, "the document is %d bytes, above the limit document.fileSize", len(content))
	}
	tree, err := jcs.Parse(content, int(lim.Get(limits.DocumentJSONDepth)))
	if err != nil {
		return document{}, plxerr.Wrap(plxerr.InvalidFormat, err, "the content is not valid JSON")
	}
	return s.documentOf(kind, tree)
}

// documentOf is parseDocument for a tree already parsed.
func (s *Service) documentOf(kind Kind, tree any) (document, error) {
	obj, ok := tree.(map[string]any)
	if !ok || obj["kind"] != string(kind) {
		return document{}, plxerr.New(plxerr.InvalidFormat, "the content is not a %s document", kind)
	}
	dk := schema.KindTemplate
	if kind == KindComponent {
		dk = schema.KindComponent
	}
	for _, d := range s.validator.Validate(dk, tree, "library") {
		if d.Severity == plxerr.SeverityError {
			return document{}, plxerr.New(plxerr.InvalidFormat, "the %s document is invalid at %s: %s", kind, d.Path, d.Message)
		}
	}
	canon, err := jcs.Marshal(tree)
	if err != nil {
		return document{}, plxerr.Wrap(plxerr.InvalidFormat, err, "the content cannot be written in canonical form")
	}
	doc := document{tree: obj, canon: canon, findings: Scan(tree)}
	doc.id, _ = obj["id"].(string)
	doc.key, _ = obj["key"].(string)
	doc.name, _ = obj["name"].(string)
	return doc, nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, it Item, before, after []byte, detail string) error {
	e := audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action, TargetKind: "library_" + string(it.Kind), TargetID: it.ID, Detail: detail,
	}
	if before != nil {
		e.BeforeHash = hashHex(before)
	}
	if after != nil {
		e.AfterHash = hashHex(after)
	}
	if _, err := s.o.Audit.Append(ctx, tx, e); err != nil {
		return fmt.Errorf("library: %w", err)
	}
	return nil
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}
