// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package library

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// FileFormat names a library file, and FileVersion is the version of its
// layout. A file is one canonical JSON object (RFC 8785): the same version of
// the same item always exports to the same bytes (SCH-006).
const (
	FileFormat  = "plux-library"
	FileVersion = 1
)

// Export returns a version of an item as a library file and the name to save
// it under. A version of zero is the newest. The file holds no identifier of
// the organisation, the author or the time, so that it is the same wherever
// it is exported from.
func (s *Service) Export(ctx context.Context, p auth.Principal, kind Kind, id string, version int32) ([]byte, string, error) {
	if err := authorize(p, auth.PluginRead); err != nil {
		return nil, "", err
	}
	var file []byte
	var name string
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		it, err := load(ctx, q, p, kind, id)
		if err != nil {
			return err
		}
		if version == 0 {
			version = it.HeadVersion
		}
		v, err := q.GetLibraryVersion(ctx, dbgen.GetLibraryVersionParams{ItemID: storage.MustUUID(it.ID), Version: version})
		if err != nil {
			return failure(err, "library version")
		}
		lim, err := s.limitsFor(ctx, tx, p)
		if err != nil {
			return err
		}
		tree, err := jcs.Parse(v.Content, int(lim.Get(limits.DocumentJSONDepth)))
		if err != nil {
			return fmt.Errorf("library: a stored version is not JSON: %w", err)
		}
		obj := map[string]any{
			"format": FileFormat, "formatVersion": float64(FileVersion), "kind": string(it.Kind), "key": it.Key, "name": it.Name,
			"description": it.Description, "contentSha256": hex.EncodeToString(v.ContentSha256), "content": tree,
		}
		if it.TemplateKind != "" {
			obj["templateKind"] = it.TemplateKind
		}
		if file, err = jcs.Marshal(obj); err != nil {
			return fmt.Errorf("library: write a library file: %w", err)
		}
		name = it.Key + ".plux-" + string(it.Kind) + ".json"
		return nil
	})
	return file, name, err
}

// Import makes a private item from a library file. The file is untrusted: it
// is parsed strictly within the registry's limits, its layout, hash, kind and
// document are all checked, and nothing in it names the organisation, author
// or visibility of the result. key replaces the file's key, and a fresh
// document identifier is given, when the organisation already has the key
// or the document.
func (s *Service) Import(ctx context.Context, p auth.Principal, kind Kind, file []byte, key string) (Item, Version, error) {
	if err := authorize(p, auth.PluginEdit); err != nil {
		return Item{}, Version{}, err
	}
	var item Item
	var ver Version
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		lim, err := s.limitsFor(ctx, tx, p)
		if err != nil {
			return err
		}
		doc, d, err := s.readFile(kind, file, lim)
		if err != nil {
			return err
		}
		if key != "" && key != doc.key {
			if doc, err = s.rekey(doc, key, kind); err != nil {
				return err
			}
			d.Key, d.Content = key, doc.canon
		}
		item, ver, err = s.insert(ctx, tx, p, kind, d, doc, lim, true)
		return err
	})
	return item, ver, err
}

// readFile checks a library file and returns the document in it and the
// draft it makes.
func (s *Service) readFile(kind Kind, file []byte, lim limits.Set) (document, Draft, error) {
	// A file wraps one document and a few lines of metadata.
	if int64(len(file)) > lim.Get(limits.DocumentFileSize)+64<<10 {
		return document{}, Draft{}, plxerr.New(plxerr.LimitExceeded, "the library file is %d bytes, above the limit document.fileSize", len(file))
	}
	tree, err := jcs.Parse(file, int(lim.Get(limits.DocumentJSONDepth)))
	if err != nil {
		return document{}, Draft{}, plxerr.Wrap(plxerr.InvalidFormat, err, "the file is not valid JSON")
	}
	obj, ok := tree.(map[string]any)
	if !ok || obj["format"] != FileFormat {
		return document{}, Draft{}, plxerr.New(plxerr.InvalidFormat, "the file is not a Plux library file")
	}
	if v, ok := obj["formatVersion"].(json.Number); !ok || v.String() != strconv.Itoa(FileVersion) {
		return document{}, Draft{}, plxerr.New(plxerr.InvalidFormat, "the library file has format version %v, this server reads %d", obj["formatVersion"], FileVersion)
	}
	if obj["kind"] != string(kind) {
		return document{}, Draft{}, plxerr.New(plxerr.InvalidFormat, "the file holds a %v, not a %s", obj["kind"], kind)
	}
	doc, err := s.documentOf(kind, obj["content"])
	if err != nil {
		return document{}, Draft{}, err
	}
	if want, _ := obj["contentSha256"].(string); want != hex.EncodeToString(sha256Sum(doc.canon)) { //nolint:errcheck // a missing hash fails the comparison
		return document{}, Draft{}, plxerr.New(plxerr.InvalidFormat, "the file's content does not match its hash")
	}
	d := Draft{Key: doc.key, Content: doc.canon}
	d.Name, _ = obj["name"].(string)
	d.Description, _ = obj["description"].(string)
	d.TemplateKind, _ = obj["templateKind"].(string)
	return doc, d, nil
}

// rekey gives a document another key and a fresh identifier, so that the
// copy is a different component from the one it was exported from.
func (s *Service) rekey(doc document, key string, kind Kind) (document, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return document{}, err //nolint:wrapcheck // an identifier failure
	}
	tree := doc.tree
	tree["key"], tree["id"] = key, id
	return s.documentOf(kind, tree)
}
