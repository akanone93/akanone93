// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// libraryID makes a UUIDv7-shaped document identifier from a number.
func libraryID(n int) string {
	return fmt.Sprintf("01a0c450-6c00-7%03x-8000-%012x", n%4096, n)
}

// libraryDocOf is the document an item was made from, which a revision
// keeps the identifier and key of.
func (f *matrixFixture) libraryDocOf(_, id string) string { return f.libraryDocs[id] }

// newLibraryDoc is a template or component document that no earlier one
// shares an identifier or key with.
func (f *matrixFixture) newLibraryDoc(kind string) string {
	f.count++
	n := f.count + 1000
	key := fmt.Sprintf("lib%d", n)
	root := fmt.Sprintf(`{"id":%q,"type":"Text","props":{"data":"Hi"}}`, libraryID(n+5000))
	if kind == "template" {
		return fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"template","id":%q,"key":%q,"name":"Lib","category":"display","visibility":"private","root":%s}`, libraryID(n), key, root)
	}
	return fmt.Sprintf(`{"schemaVersion":"1.0.0","kind":"component","id":%q,"key":%q,"name":"Lib","version":1,"props":[{"name":"title","type":"string"}],"root":%s}`, libraryID(n), key, root)
}

// libraryDoc is a new document of the kind.
func (f *matrixFixture) libraryDoc(kind string) string { return f.newLibraryDoc(kind) }

// libraryItem returns an item of the kind that the owner made and shared with
// the organisation, made on first use.
func (f *matrixFixture) libraryItem(t *testing.T, kind string) string {
	t.Helper()
	if id := f.library[kind]; id != "" {
		return id
	}
	ctx := context.Background()
	doc := f.libraryDoc(kind)
	var id string
	draft := &pluxv1.LibraryDraft{Content: []byte(doc)}
	if kind == "template" {
		id = must(f.w.template.CreateTemplate(ctx, req(f.owner(), &pluxv1.CreateTemplateRequest{Draft: draft})))(t).GetItem().GetId()
		must(f.w.template.SetTemplateVisibility(ctx, req(f.owner(), &pluxv1.SetTemplateVisibilityRequest{Id: id, Visibility: "organization"})))(t)
	} else {
		id = must(f.w.comp.CreateComponent(ctx, req(f.owner(), &pluxv1.CreateComponentRequest{Draft: draft})))(t).GetItem().GetId()
		must(f.w.comp.SetComponentVisibility(ctx, req(f.owner(), &pluxv1.SetComponentVisibilityRequest{Id: id, Visibility: "organization"})))(t)
	}
	if f.library == nil {
		f.library = map[string]string{}
		f.libraryDocs = map[string]string{}
	}
	f.library[kind] = id
	f.libraryDocs[id] = doc
	return id
}

var _ proto.Message = (*pluxv1.LibraryDraft)(nil)
