// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package library

import (
	"context"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var keyForm = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Create makes a private item with its first version (STU-080). Anyone who
// may edit plugins may author library content.
func (s *Service) Create(ctx context.Context, p auth.Principal, kind Kind, d Draft) (Item, Version, error) {
	if err := authorize(p, auth.PluginEdit); err != nil {
		return Item{}, Version{}, err
	}
	var item Item
	var ver Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) (err error) {
		item, ver, err = s.create(ctx, tx, p, kind, d, false)
		return err
	})
	return item, ver, err
}

// create inserts an item and its first version in the caller's transaction.
// imported says the document is the content of a library file, for the audit
// trail.
func (s *Service) create(ctx context.Context, tx pgx.Tx, p auth.Principal, kind Kind, d Draft, imported bool) (Item, Version, error) {
	lim, err := s.limitsFor(ctx, tx, p)
	if err != nil {
		return Item{}, Version{}, err
	}
	doc, err := s.parseDocument(kind, d.Content, lim)
	if err != nil {
		return Item{}, Version{}, err
	}
	return s.insert(ctx, tx, p, kind, d, doc, lim, imported)
}

func (s *Service) insert(ctx context.Context, tx pgx.Tx, p auth.Principal, kind Kind, d Draft, doc document, lim limits.Set, imported bool) (Item, Version, error) {
	key := d.Key
	if key == "" {
		key = doc.key
	}
	if !keyForm.MatchString(key) || key != doc.key {
		return Item{}, Version{}, plxerr.New(plxerr.InvalidFormat, "the key must be lower-case letters, digits and hyphens and equal the document's key")
	}
	templateKind := ""
	if kind == KindTemplate {
		if templateKind = d.TemplateKind; templateKind == "" {
			templateKind = "page"
		}
		if templateKind != "page" && templateKind != "flow" {
			return Item{}, Version{}, plxerr.New(plxerr.InvalidEnumValue, "a template's kind is \"page\" or \"flow\"")
		}
	}
	name := orDefault(d.Name, doc.name)
	description := d.Description
	if description == "" {
		description, _ = doc.tree["description"].(string)
	}
	q := dbgen.New(tx)
	org, err := parseID(p.OrganizationID, "organization")
	if err != nil {
		return Item{}, Version{}, err
	}
	count, err := q.CountLibraryItems(ctx, org)
	if err != nil {
		return Item{}, Version{}, failure(err, "library item")
	}
	if max := lim.Get(limits.LibraryItemsPerOrganization); count >= max {
		return Item{}, Version{}, plxerr.New(plxerr.LimitExceeded, "the organisation has %d library items, the limit library.itemsPerOrganization", max)
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Item{}, Version{}, err //nolint:wrapcheck // an identifier failure
	}
	row, err := q.InsertLibraryItem(ctx, dbgen.InsertLibraryItemParams{
		ID: storage.MustUUID(id), OrganizationID: org, Kind: string(kind), Key: key, DocumentID: doc.id, Name: name,
		Description: description, TemplateKind: templateKind, CreatedBy: p.UserID, CreatedByName: p.Actor().Display,
	})
	if err != nil {
		return Item{}, Version{}, failure(err, string(kind))
	}
	vrow, err := q.InsertLibraryVersion(ctx, versionParams(row.ID, org, 1, doc, d.Note, p))
	if err != nil {
		return Item{}, Version{}, failure(err, "library version")
	}
	item := itemOf(row)
	detail := ""
	if imported {
		detail = "imported from a library file"
	}
	if err := s.record(ctx, tx, p, audit.LibraryItemCreated, item, nil, doc.canon, detail); err != nil {
		return Item{}, Version{}, err
	}
	return item, versionOf(vrow), nil
}

func versionParams(itemID, org pgtype.UUID, number int32, doc document, note string, p auth.Principal) dbgen.InsertLibraryVersionParams {
	return dbgen.InsertLibraryVersionParams{
		ItemID: itemID, OrganizationID: org, Version: number, Content: doc.canon, ContentSha256: sha256Sum(doc.canon),
		Note: note, CreatedBy: p.UserID, CreatedByName: p.Actor().Display,
	}
}

func orDefault(s, d string) string {
	if s != "" {
		return s
	}
	return d
}

// Revise writes the next immutable version of an item (STU-080). The author
// may revise it; anyone else needs library.manage. Changing a public item
// also needs library.publish, and the new content must pass the public scan,
// since it is public the moment it is saved. expected is the head version the
// caller revised from.
func (s *Service) Revise(ctx context.Context, p auth.Principal, kind Kind, id string, expected int32, d Draft) (Item, Version, error) {
	var item Item
	var ver Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) (err error) {
		item, ver, err = s.revise(ctx, tx, p, kind, id, expected, d)
		return err
	})
	return item, ver, err
}

func (s *Service) revise(ctx context.Context, tx pgx.Tx, p auth.Principal, kind Kind, id string, expected int32, d Draft) (Item, Version, error) {
	q := dbgen.New(tx)
	cur, err := lockItem(ctx, q, p, kind, id)
	if err != nil {
		return Item{}, Version{}, err
	}
	if err := mayEdit(p, cur); err != nil {
		return Item{}, Version{}, err
	}
	if cur.Visibility == Public {
		if err := authorize(p, auth.LibraryPublish); err != nil {
			return Item{}, Version{}, err
		}
	}
	if cur.HeadVersion != expected {
		return Item{}, Version{}, plxerr.New(plxerr.RevisionConflict, "the %s is at version %d, not %d", kind, cur.HeadVersion, expected)
	}
	lim, err := s.limitsFor(ctx, tx, p)
	if err != nil {
		return Item{}, Version{}, err
	}
	doc, err := s.parseDocument(kind, d.Content, lim)
	if err != nil {
		return Item{}, Version{}, err
	}
	if err := checkRevision(cur, doc, lim); err != nil {
		return Item{}, Version{}, err
	}
	uid := storage.MustUUID(cur.ID)
	next := cur.HeadVersion + 1
	vrow, err := q.InsertLibraryVersion(ctx, versionParams(uid, storage.MustUUID(p.OrganizationID), next, doc, d.Note, p))
	if err != nil {
		return Item{}, Version{}, failure(err, "library version")
	}
	updated, err := q.UpdateLibraryItem(ctx, dbgen.UpdateLibraryItemParams{
		ID: uid, Name: orDefault(d.Name, cur.Name), Description: orDefault(d.Description, cur.Description), Visibility: cur.Visibility, HeadVersion: next,
	})
	if err != nil {
		return Item{}, Version{}, failure(err, string(kind))
	}
	prev, err := q.GetLibraryVersion(ctx, dbgen.GetLibraryVersionParams{ItemID: uid, Version: cur.HeadVersion})
	if err != nil {
		return Item{}, Version{}, failure(err, "library version")
	}
	item := itemOf(updated)
	return item, versionOf(vrow), s.record(ctx, tx, p, audit.LibraryVersionAdded, item, prev.Content, doc.canon, "")
}

// checkRevision refuses a document that is not the next version of the item:
// another document, content that cannot be public while the item is, or a
// version beyond the limit.
func checkRevision(cur Item, doc document, lim limits.Set) error {
	if doc.id != cur.DocumentID || doc.key != cur.Key {
		return plxerr.New(plxerr.InvalidFormat, "a new version keeps the document's id and key")
	}
	if cur.Visibility == Public && len(doc.findings) > 0 {
		return scanError(doc.findings)
	}
	if int64(cur.HeadVersion) >= lim.Get(limits.LibraryVersionsPerItem) {
		return plxerr.New(plxerr.LimitExceeded, "the %s has %d versions, the limit library.versionsPerItem", cur.Kind, cur.HeadVersion)
	}
	return nil
}

// lockItem reads an item of the principal's organisation for update.
func lockItem(ctx context.Context, q *dbgen.Queries, p auth.Principal, kind Kind, id string) (Item, error) {
	uid, err := parseID(id, string(kind))
	if err != nil {
		return Item{}, err
	}
	org, err := parseID(p.OrganizationID, "organization")
	if err != nil {
		return Item{}, err
	}
	row, err := q.GetLibraryItemForUpdate(ctx, dbgen.GetLibraryItemForUpdateParams{ID: uid, OrganizationID: org, Actor: p.UserID})
	if err != nil {
		return Item{}, failure(err, string(kind))
	}
	it := itemOf(row)
	if it.Kind != kind {
		return Item{}, plxerr.New(plxerr.ResourceNotFound, "no such %s", kind)
	}
	return it, nil
}

// Versions returns an item with all its versions, newest first.
func (s *Service) Versions(ctx context.Context, p auth.Principal, kind Kind, id string) (Item, []Version, error) {
	if err := authorize(p, auth.PluginRead); err != nil {
		return Item{}, nil, err
	}
	var item Item
	var out []Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if item, err = load(ctx, q, p, kind, id); err != nil {
			return err
		}
		rows, err := q.ListLibraryVersions(ctx, storage.MustUUID(item.ID))
		if err != nil {
			return failure(err, "library version")
		}
		for _, r := range rows {
			out = append(out, versionOf(r))
		}
		return nil
	})
	return item, out, err
}

// ListFilter narrows a listing.
type ListFilter struct {
	Visibility, Query string
	// After is the key and identifier of the last item of the previous page.
	AfterKey, AfterID string
}

// List lists the items of a kind the principal may see: the organisation's
// shared ones, their own private ones and every public one (STU-086).
func (s *Service) List(ctx context.Context, p auth.Principal, kind Kind, f ListFilter, size int32) ([]Item, error) {
	if err := authorize(p, auth.PluginRead); err != nil {
		return nil, err
	}
	if f.Visibility != "" && !validVisibility(f.Visibility) {
		return nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a visibility", f.Visibility)
	}
	params := dbgen.ListLibraryItemsParams{Kind: string(kind), Actor: p.UserID, Visibility: f.Visibility, Query: f.Query, AfterKey: f.AfterKey, PageSize: size}
	if f.AfterID != "" {
		id, err := parseID(f.AfterID, "page token")
		if err != nil {
			return nil, err
		}
		params.AfterID = id
	}
	var out []Item
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListLibraryItems(ctx, params)
		if err != nil {
			return failure(err, "library item")
		}
		for _, r := range rows {
			out = append(out, itemOf(r))
		}
		return nil
	})
	return out, err
}

func validVisibility(v string) bool { return v == Private || v == Organization || v == Public }

// SetVisibility changes who can see an item (STU-081). Making an item
// public, or taking it back from public, needs library.publish as well as the
// right to edit it, and the newest version must pass the public scan first.
func (s *Service) SetVisibility(ctx context.Context, p auth.Principal, kind Kind, id, visibility string) (Item, error) {
	if !validVisibility(visibility) {
		return Item{}, plxerr.New(plxerr.InvalidEnumValue, "%q is not a visibility", visibility)
	}
	if visibility == Public {
		if err := authorize(p, auth.LibraryPublish); err != nil {
			return Item{}, err
		}
	}
	var out Item
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) (err error) {
		out, err = s.setVisibility(ctx, tx, p, kind, id, visibility)
		return err
	})
	return out, err
}

func (s *Service) setVisibility(ctx context.Context, tx pgx.Tx, p auth.Principal, kind Kind, id, visibility string) (Item, error) {
	q := dbgen.New(tx)
	cur, err := lockItem(ctx, q, p, kind, id)
	if err != nil {
		return Item{}, err
	}
	if err := mayEdit(p, cur); err != nil {
		return Item{}, err
	}
	if cur.Visibility == Public {
		if err := authorize(p, auth.LibraryPublish); err != nil {
			return Item{}, err
		}
	}
	if cur.Visibility == visibility {
		return cur, nil
	}
	uid := storage.MustUUID(cur.ID)
	head, err := q.GetLibraryVersion(ctx, dbgen.GetLibraryVersionParams{ItemID: uid, Version: cur.HeadVersion})
	if err != nil {
		return Item{}, failure(err, "library version")
	}
	if visibility == Public {
		lim, err := s.limitsFor(ctx, tx, p)
		if err != nil {
			return Item{}, err
		}
		if err := s.publicScan(kind, head.Content, lim); err != nil {
			return Item{}, err
		}
	}
	row, err := q.UpdateLibraryItem(ctx, dbgen.UpdateLibraryItemParams{
		ID: uid, Name: cur.Name, Description: cur.Description, Visibility: visibility, HeadVersion: cur.HeadVersion,
	})
	if err != nil {
		return Item{}, failure(err, string(kind))
	}
	out := itemOf(row)
	return out, s.record(ctx, tx, p, audit.LibraryVisibilitySet, out, nil, head.Content, cur.Visibility+" -> "+visibility)
}

// publicScan scans a stored version, returning the refusal when it finds
// anything.
func (s *Service) publicScan(kind Kind, content []byte, lim limits.Set) error {
	doc, err := s.parseDocument(kind, content, lim)
	if err != nil {
		return err
	}
	if len(doc.findings) > 0 {
		return scanError(doc.findings)
	}
	return nil
}
