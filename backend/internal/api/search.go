// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/search"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Search serves SearchService (SRV-061).
type Search struct{ h *Handlers }

var _ pluxv1connect.SearchServiceHandler = Search{}

// Search returns the handler.
func (h *Handlers) Search() Search { return Search{h: h} }

// Search finds what the caller may read. The page token binds the words and
// kinds it was issued for and carries the offset of the next page.
func (s Search) Search(ctx context.Context, req *connect.Request[pluxv1.SearchRequest]) (*connect.Response[pluxv1.SearchResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.SearchResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		scope := p.OrganizationID + "\x00" + m.GetQuery() + "\x00" + strings.Join(m.GetKinds(), ",")
		page := m.GetPage()
		after, _, err := s.h.Pages.Request(req.Spec().Procedure, scope, page)
		if err != nil {
			return nil, err
		}
		res, err := s.h.Searches.Search(ctx, p, search.Query{
			Text: m.GetQuery(), Kinds: m.GetKinds(), PageSize: page.GetPageSize(), Offset: int32(after.Sequence), //nolint:gosec // tokens are issued by this server with bounded offsets
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.SearchResponse{Page: &pluxv1.PageResult{}}
		for _, h := range res.Hits {
			out.Hits = append(out.Hits, &pluxv1.SearchHit{
				Kind: h.Kind, Id: h.ID, AppId: h.AppID, Name: h.Name, Path: h.Path, Snippet: h.Snippet, Rank: h.Rank,
			})
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, scope, page, len(res.Hits), res.PageSize,
			storage.Cursor{Sequence: after.Sequence + int64(len(res.Hits))})
		return out, nil
	})
}
