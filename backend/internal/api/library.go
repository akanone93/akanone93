// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/library"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// The library's authoring calls of TemplateService and ComponentService
// (STU-080 to STU-086, ADR-0098). Both services work on the same store; a
// handler only converts, and the library service decides.

func libraryItemProto(it library.Item) *pluxv1.LibraryItem {
	return &pluxv1.LibraryItem{
		Id: it.ID, Kind: string(it.Kind), Key: it.Key, Name: it.Name, Description: it.Description, TemplateKind: it.TemplateKind,
		Visibility: it.Visibility, HeadVersion: it.HeadVersion, DocumentId: it.DocumentID, OrganizationId: it.OrganizationID,
		CreatedBy: it.CreatedBy, CreatedByName: it.CreatedByName, CreatedAt: timestamppb.New(it.CreatedAt), UpdatedAt: timestamppb.New(it.UpdatedAt),
	}
}

func libraryVersionProto(v library.Version) *pluxv1.LibraryVersion {
	return &pluxv1.LibraryVersion{
		ItemId: v.ItemID, Version: v.Number, Content: v.Content, ContentSha256: v.ContentSHA256, Note: v.Note,
		CreatedByName: v.CreatedByName, CreatedAt: timestamppb.New(v.CreatedAt),
	}
}

func libraryDraft(d *pluxv1.LibraryDraft) library.Draft {
	return library.Draft{
		Key: d.GetKey(), Name: d.GetName(), Description: d.GetDescription(), TemplateKind: d.GetTemplateKind(),
		Content: d.GetContent(), Note: d.GetNote(),
	}
}

func adoptionProto(a library.Adoption) *pluxv1.Adoption {
	return &pluxv1.Adoption{AppId: a.AppID, ItemId: a.ItemID, Version: a.Version, AdoptedBy: a.AdoptedBy, AdoptedAt: timestamppb.New(a.AdoptedAt)}
}

// libraryService is the library service, or the refusal of an installation
// that has none.
func (h *Handlers) libraryService() (*library.Service, error) {
	if h.Library == nil {
		return nil, plxerr.New(plxerr.UpstreamUnavailable, "the library is not available on this installation")
	}
	return h.Library, nil
}

func (h *Handlers) libraryCreate(ctx context.Context, header http.Header, kind library.Kind, d *pluxv1.LibraryDraft) (*pluxv1.LibraryItem, *pluxv1.LibraryVersion, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, nil, err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, nil, err
	}
	it, v, err := svc.Create(ctx, p, kind, libraryDraft(d))
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	return libraryItemProto(it), libraryVersionProto(v), nil
}

func (h *Handlers) libraryRevise(ctx context.Context, header http.Header, kind library.Kind, id string, expected int32, d *pluxv1.LibraryDraft) (*pluxv1.LibraryItem, *pluxv1.LibraryVersion, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, nil, err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, nil, err
	}
	it, v, err := svc.Revise(ctx, p, kind, id, expected, libraryDraft(d))
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	return libraryItemProto(it), libraryVersionProto(v), nil
}

func (h *Handlers) libraryVersions(ctx context.Context, header http.Header, kind library.Kind, id string) (*pluxv1.LibraryItem, []*pluxv1.LibraryVersion, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, nil, err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, nil, err
	}
	it, vs, err := svc.Versions(ctx, p, kind, id)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	out := make([]*pluxv1.LibraryVersion, 0, len(vs))
	for _, v := range vs {
		out = append(out, libraryVersionProto(v))
	}
	return libraryItemProto(it), out, nil
}

func (h *Handlers) libraryList(ctx context.Context, header http.Header, procedure string, kind library.Kind, visibility, query string, page *pluxv1.Page) ([]*pluxv1.LibraryItem, *pluxv1.PageResult, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, nil, err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, nil, err
	}
	after, size, err := h.Pages.Request(procedure, p.OrganizationID, page)
	if err != nil {
		return nil, nil, err
	}
	items, err := svc.List(ctx, p, kind, library.ListFilter{Visibility: visibility, Query: query, AfterKey: after.Key, AfterID: after.ID}, size)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	out := make([]*pluxv1.LibraryItem, 0, len(items))
	var last storage.Cursor
	for _, it := range items {
		last = storage.Cursor{Key: it.Key, ID: it.ID}
		out = append(out, libraryItemProto(it))
	}
	return out, &pluxv1.PageResult{NextPageToken: h.Pages.Next(procedure, p.OrganizationID, page, len(items), size, last)}, nil
}

func (h *Handlers) librarySetVisibility(ctx context.Context, header http.Header, kind library.Kind, id, visibility string) (*pluxv1.LibraryItem, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, err
	}
	it, err := svc.SetVisibility(ctx, p, kind, id, visibility)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return libraryItemProto(it), nil
}

func (h *Handlers) libraryExport(ctx context.Context, header http.Header, kind library.Kind, id string, version int32) ([]byte, string, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, "", err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, "", err
	}
	return svc.Export(ctx, p, kind, id, version) //nolint:wrapcheck // a domain error
}

func (h *Handlers) libraryImport(ctx context.Context, header http.Header, kind library.Kind, file []byte, key string) (*pluxv1.LibraryItem, *pluxv1.LibraryVersion, error) {
	svc, err := h.libraryService()
	if err != nil {
		return nil, nil, err
	}
	p, err := h.principal(ctx, header, "")
	if err != nil {
		return nil, nil, err
	}
	it, v, err := svc.Import(ctx, p, kind, file, key)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	return libraryItemProto(it), libraryVersionProto(v), nil
}

// CreateTemplate makes a private template with its first version.
func (s Template) CreateTemplate(ctx context.Context, req *connect.Request[pluxv1.CreateTemplateRequest]) (*connect.Response[pluxv1.CreateTemplateResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateTemplateResponse, error) {
		it, v, err := s.h.libraryCreate(ctx, req.Header(), library.KindTemplate, req.Msg.GetDraft())
		return &pluxv1.CreateTemplateResponse{Item: it, Version: v}, err
	})
}

// ReviseTemplate writes a new version of a template.
func (s Template) ReviseTemplate(ctx context.Context, req *connect.Request[pluxv1.ReviseTemplateRequest]) (*connect.Response[pluxv1.ReviseTemplateResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ReviseTemplateResponse, error) {
		m := req.Msg
		it, v, err := s.h.libraryRevise(ctx, req.Header(), library.KindTemplate, m.GetId(), m.GetExpectedVersion(), m.GetDraft())
		return &pluxv1.ReviseTemplateResponse{Item: it, Version: v}, err
	})
}

// ListTemplateVersions returns a template with its versions.
func (s Template) ListTemplateVersions(ctx context.Context, req *connect.Request[pluxv1.ListTemplateVersionsRequest]) (*connect.Response[pluxv1.ListTemplateVersionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListTemplateVersionsResponse, error) {
		it, vs, err := s.h.libraryVersions(ctx, req.Header(), library.KindTemplate, req.Msg.GetId())
		return &pluxv1.ListTemplateVersionsResponse{Item: it, Versions: vs}, err
	})
}

// ListLibraryTemplates lists the templates the caller may see.
func (s Template) ListLibraryTemplates(ctx context.Context, req *connect.Request[pluxv1.ListLibraryTemplatesRequest]) (*connect.Response[pluxv1.ListLibraryTemplatesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListLibraryTemplatesResponse, error) {
		m := req.Msg
		items, page, err := s.h.libraryList(ctx, req.Header(), req.Spec().Procedure, library.KindTemplate, m.GetVisibility(), m.GetQuery(), m.GetPage())
		return &pluxv1.ListLibraryTemplatesResponse{Items: items, Page: page}, err
	})
}

// SetTemplateVisibility changes who can see a template.
func (s Template) SetTemplateVisibility(ctx context.Context, req *connect.Request[pluxv1.SetTemplateVisibilityRequest]) (*connect.Response[pluxv1.SetTemplateVisibilityResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetTemplateVisibilityResponse, error) {
		it, err := s.h.librarySetVisibility(ctx, req.Header(), library.KindTemplate, req.Msg.GetId(), req.Msg.GetVisibility())
		return &pluxv1.SetTemplateVisibilityResponse{Item: it}, err
	})
}

// ExportTemplate returns a template version as a library file.
func (s Template) ExportTemplate(ctx context.Context, req *connect.Request[pluxv1.ExportTemplateRequest]) (*connect.Response[pluxv1.ExportTemplateResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExportTemplateResponse, error) {
		file, name, err := s.h.libraryExport(ctx, req.Header(), library.KindTemplate, req.Msg.GetId(), req.Msg.GetVersion())
		return &pluxv1.ExportTemplateResponse{File: file, FileName: name}, err
	})
}

// ImportTemplate makes a private template from a library file.
func (s Template) ImportTemplate(ctx context.Context, req *connect.Request[pluxv1.ImportTemplateRequest]) (*connect.Response[pluxv1.ImportTemplateResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ImportTemplateResponse, error) {
		it, v, err := s.h.libraryImport(ctx, req.Header(), library.KindTemplate, req.Msg.GetFile(), req.Msg.GetKey())
		return &pluxv1.ImportTemplateResponse{Item: it, Version: v}, err
	})
}

// CreateComponent makes a private component with its first version.
func (s Component) CreateComponent(ctx context.Context, req *connect.Request[pluxv1.CreateComponentRequest]) (*connect.Response[pluxv1.CreateComponentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateComponentResponse, error) {
		it, v, err := s.h.libraryCreate(ctx, req.Header(), library.KindComponent, req.Msg.GetDraft())
		return &pluxv1.CreateComponentResponse{Item: it, Version: v}, err
	})
}

// ReviseComponent writes a new version of a component.
func (s Component) ReviseComponent(ctx context.Context, req *connect.Request[pluxv1.ReviseComponentRequest]) (*connect.Response[pluxv1.ReviseComponentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ReviseComponentResponse, error) {
		m := req.Msg
		it, v, err := s.h.libraryRevise(ctx, req.Header(), library.KindComponent, m.GetId(), m.GetExpectedVersion(), m.GetDraft())
		return &pluxv1.ReviseComponentResponse{Item: it, Version: v}, err
	})
}

// ListComponentVersions returns a component with its versions.
func (s Component) ListComponentVersions(ctx context.Context, req *connect.Request[pluxv1.ListComponentVersionsRequest]) (*connect.Response[pluxv1.ListComponentVersionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListComponentVersionsResponse, error) {
		it, vs, err := s.h.libraryVersions(ctx, req.Header(), library.KindComponent, req.Msg.GetId())
		return &pluxv1.ListComponentVersionsResponse{Item: it, Versions: vs}, err
	})
}

// ListLibraryComponents lists the components the caller may see.
func (s Component) ListLibraryComponents(ctx context.Context, req *connect.Request[pluxv1.ListLibraryComponentsRequest]) (*connect.Response[pluxv1.ListLibraryComponentsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListLibraryComponentsResponse, error) {
		m := req.Msg
		items, page, err := s.h.libraryList(ctx, req.Header(), req.Spec().Procedure, library.KindComponent, m.GetVisibility(), m.GetQuery(), m.GetPage())
		return &pluxv1.ListLibraryComponentsResponse{Items: items, Page: page}, err
	})
}

// SetComponentVisibility changes who can see a component.
func (s Component) SetComponentVisibility(ctx context.Context, req *connect.Request[pluxv1.SetComponentVisibilityRequest]) (*connect.Response[pluxv1.SetComponentVisibilityResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetComponentVisibilityResponse, error) {
		it, err := s.h.librarySetVisibility(ctx, req.Header(), library.KindComponent, req.Msg.GetId(), req.Msg.GetVisibility())
		return &pluxv1.SetComponentVisibilityResponse{Item: it}, err
	})
}

// ExportComponent returns a component version as a library file.
func (s Component) ExportComponent(ctx context.Context, req *connect.Request[pluxv1.ExportComponentRequest]) (*connect.Response[pluxv1.ExportComponentResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExportComponentResponse, error) {
		file, name, err := s.h.libraryExport(ctx, req.Header(), library.KindComponent, req.Msg.GetId(), req.Msg.GetVersion())
		return &pluxv1.ExportComponentResponse{File: file, FileName: name}, err
	})
}

// ImportComponent makes a private component from a library file.
func (s Component) ImportComponent(ctx context.Context, req *connect.Request[pluxv1.ImportComponentRequest]) (*connect.Response[pluxv1.ImportComponentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ImportComponentResponse, error) {
		it, v, err := s.h.libraryImport(ctx, req.Header(), library.KindComponent, req.Msg.GetFile(), req.Msg.GetKey())
		return &pluxv1.ImportComponentResponse{Item: it, Version: v}, err
	})
}

// AdoptComponentVersion records the version an app uses and reports the
// instances it no longer fits.
func (s Component) AdoptComponentVersion(ctx context.Context, req *connect.Request[pluxv1.AdoptComponentVersionRequest]) (*connect.Response[pluxv1.AdoptComponentVersionResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.AdoptComponentVersionResponse, error) {
		svc, err := s.h.libraryService()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		a, conflicts, err := svc.Adopt(ctx, p, m.GetAppId(), m.GetId(), m.GetVersion())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.AdoptComponentVersionResponse{Adoption: adoptionProto(a)}
		for _, c := range conflicts {
			out.Conflicts = append(out.Conflicts, &pluxv1.AdoptionConflict{File: c.File, Path: c.Path, RemovedProps: c.RemovedProps, RemovedSlots: c.RemovedSlots})
		}
		return out, nil
	})
}

// ListComponentAdoptions lists the versions an app adopted.
func (s Component) ListComponentAdoptions(ctx context.Context, req *connect.Request[pluxv1.ListComponentAdoptionsRequest]) (*connect.Response[pluxv1.ListComponentAdoptionsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListComponentAdoptionsResponse, error) {
		svc, err := s.h.libraryService()
		if err != nil {
			return nil, err
		}
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		as, err := svc.Adoptions(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListComponentAdoptionsResponse{}
		for _, a := range as {
			out.Adoptions = append(out.Adoptions, adoptionProto(a))
		}
		return out, nil
	})
}
