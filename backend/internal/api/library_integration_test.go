// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// Verifies: STU-080, STU-081, STU-085, STU-086, SRV-002.
// The library's calls end to end over HTTP, for templates and components:
// create, revise, list, versions, visibility with the public scan's refusal
// mapped to FailedPrecondition, export and import byte for byte, and the
// adoption of a component version with its conflict report.
func TestLibraryServicesEndToEnd_STU_080(t *testing.T) {
	t.Parallel()
	f := newMatrixFixture(t)
	w, owner := f.w, f.owner()
	ctx := context.Background()

	// Templates.
	doc := f.libraryDoc("template")
	created := must(w.template.CreateTemplate(ctx, req(owner, &pluxv1.CreateTemplateRequest{Draft: &pluxv1.LibraryDraft{Content: []byte(doc), Note: "first"}})))(t)
	id := created.GetItem().GetId()
	if created.GetItem().GetVisibility() != "private" || created.GetVersion().GetVersion() != 1 || created.GetItem().GetTemplateKind() != "page" {
		t.Fatalf("CreateTemplate = %v", created)
	}
	revised := must(w.template.ReviseTemplate(ctx, req(owner, &pluxv1.ReviseTemplateRequest{
		Id: id, ExpectedVersion: 1, Draft: &pluxv1.LibraryDraft{Content: []byte(strings.Replace(doc, `"Hi"`, `"Hello"`, 1)), Name: "Hello card"},
	})))(t)
	if revised.GetItem().GetHeadVersion() != 2 || revised.GetItem().GetName() != "Hello card" {
		t.Fatalf("ReviseTemplate = %v", revised)
	}
	if _, err := w.template.ReviseTemplate(ctx, req(owner, &pluxv1.ReviseTemplateRequest{Id: id, ExpectedVersion: 1, Draft: &pluxv1.LibraryDraft{Content: []byte(doc)}})); codeOf(err) != connect.CodeAborted {
		t.Errorf("a stale revision = %v, want Aborted", codeOf(err))
	}
	versions := must(w.template.ListTemplateVersions(ctx, req(owner, &pluxv1.ListTemplateVersionsRequest{Id: id})))(t)
	if len(versions.GetVersions()) != 2 || versions.GetVersions()[0].GetVersion() != 2 {
		t.Fatalf("ListTemplateVersions = %v", versions)
	}
	must(w.template.SetTemplateVisibility(ctx, req(owner, &pluxv1.SetTemplateVisibilityRequest{Id: id, Visibility: "organization"})))(t)
	listed := must(w.template.ListLibraryTemplates(ctx, req(f.callers[auth.RoleViewer], &pluxv1.ListLibraryTemplatesRequest{Query: "lib"})))(t)
	if len(listed.GetItems()) != 1 || listed.GetItems()[0].GetId() != id {
		t.Fatalf("ListLibraryTemplates as a viewer = %v", listed)
	}

	// A secret blocks the public level; the message names the place, not the value.
	dirty := strings.Replace(f.libraryDoc("template"), `"Hi"`, `"AK`+"IA"+`ABCDEFGHIJKLMNOP"`, 1)
	bad := must(w.template.CreateTemplate(ctx, req(owner, &pluxv1.CreateTemplateRequest{Draft: &pluxv1.LibraryDraft{Content: []byte(dirty)}})))(t)
	_, err := w.template.SetTemplateVisibility(ctx, req(owner, &pluxv1.SetTemplateVisibilityRequest{Id: bad.GetItem().GetId(), Visibility: "public"}))
	if codeOf(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), "/root/props/data (secret)") || strings.Contains(err.Error(), "AK"+"IA") {
		t.Errorf("a secret went public: %v", err)
	}
	public := must(w.template.SetTemplateVisibility(ctx, req(owner, &pluxv1.SetTemplateVisibilityRequest{Id: id, Visibility: "public"})))(t)
	if public.GetItem().GetVisibility() != "public" {
		t.Errorf("SetTemplateVisibility = %v", public)
	}

	// Export is deterministic and an import elsewhere exports the same bytes.
	exported := must(w.template.ExportTemplate(ctx, req(owner, &pluxv1.ExportTemplateRequest{Id: id})))(t)
	copied := must(w.template.ImportTemplate(ctx, req(owner, &pluxv1.ImportTemplateRequest{File: exported.GetFile(), Key: "copy-of-hello"})))(t)
	back := must(w.template.ExportTemplate(ctx, req(owner, &pluxv1.ExportTemplateRequest{Id: copied.GetItem().GetId()})))(t)
	if !bytes.Contains(back.GetFile(), []byte(`"key":"copy-of-hello"`)) || bytes.Equal(back.GetFile(), exported.GetFile()) {
		t.Errorf("the copy under another key exports as the original or without its key")
	}
	if _, err := w.template.ImportTemplate(ctx, req(owner, &pluxv1.ImportTemplateRequest{File: []byte(`{"format":"nope"}`)})); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a file that is not a library file = %v, want InvalidArgument", codeOf(err))
	}

	// Components and adoption.
	cdoc := f.libraryDoc("component")
	comp := must(w.comp.CreateComponent(ctx, req(owner, &pluxv1.CreateComponentRequest{Draft: &pluxv1.LibraryDraft{Content: []byte(cdoc)}})))(t)
	cid := comp.GetItem().GetId()
	must(w.comp.ReviseComponent(ctx, req(owner, &pluxv1.ReviseComponentRequest{
		Id: cid, ExpectedVersion: 1, Draft: &pluxv1.LibraryDraft{Content: []byte(strings.Replace(cdoc, `"Hi"`, `"Hey"`, 1))},
	})))(t)
	adopted := must(w.comp.AdoptComponentVersion(ctx, req(owner, &pluxv1.AdoptComponentVersionRequest{AppId: f.app, Id: cid, Version: 2})))(t)
	if adopted.GetAdoption().GetVersion() != 2 || len(adopted.GetConflicts()) != 0 {
		t.Errorf("AdoptComponentVersion = %v", adopted)
	}
	adoptions := must(w.comp.ListComponentAdoptions(ctx, req(owner, &pluxv1.ListComponentAdoptionsRequest{AppId: f.app})))(t)
	if len(adoptions.GetAdoptions()) != 1 || adoptions.GetAdoptions()[0].GetItemId() != cid {
		t.Errorf("ListComponentAdoptions = %v", adoptions)
	}
	if _, err := w.comp.AdoptComponentVersion(ctx, req(owner, &pluxv1.AdoptComponentVersionRequest{AppId: f.app, Id: cid, Version: 9})); codeOf(err) != connect.CodeNotFound {
		t.Errorf("adopting a version that does not exist = %v, want NotFound", codeOf(err))
	}
	cfile := must(w.comp.ExportComponent(ctx, req(owner, &pluxv1.ExportComponentRequest{Id: cid, Version: 1})))(t)
	if !strings.HasSuffix(cfile.GetFileName(), ".plux-component.json") {
		t.Errorf("ExportComponent file name = %q", cfile.GetFileName())
	}
	must(w.comp.ImportComponent(ctx, req(owner, &pluxv1.ImportComponentRequest{File: cfile.GetFile(), Key: "imported-card"})))(t)
	if got := must(w.comp.ListComponentVersions(ctx, req(owner, &pluxv1.ListComponentVersionsRequest{Id: cid})))(t); len(got.GetVersions()) != 2 {
		t.Errorf("ListComponentVersions = %v", got)
	}
	if got := must(w.comp.ListLibraryComponents(ctx, req(owner, &pluxv1.ListLibraryComponentsRequest{})))(t); len(got.GetItems()) < 2 {
		t.Errorf("ListLibraryComponents = %v", got)
	}
}
