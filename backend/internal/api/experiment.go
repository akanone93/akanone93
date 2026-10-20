// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Flags serves FlagService: the feature flags of an environment (ABT-006).
type Flags struct {
	pluxv1connect.UnimplementedFlagServiceHandler
	h *Handlers
}

var _ pluxv1connect.FlagServiceHandler = Flags{}

// Flags returns the FlagService handler.
func (h *Handlers) Flags() Flags { return Flags{h: h} }

// flagTypes maps the wire enumeration to the types of a flag.
var flagTypes = map[pluxv1.FlagType]controldoc.FlagType{
	pluxv1.FlagType_FLAG_TYPE_BOOL:   controldoc.FlagBool,
	pluxv1.FlagType_FLAG_TYPE_NUMBER: controldoc.FlagNumber,
	pluxv1.FlagType_FLAG_TYPE_STRING: controldoc.FlagString,
	pluxv1.FlagType_FLAG_TYPE_JSON:   controldoc.FlagJSON,
}

// protoFlagType is the inverse of flagTypes.
func protoFlagType(t controldoc.FlagType) pluxv1.FlagType {
	for wire, typ := range flagTypes {
		if typ == t {
			return wire
		}
	}
	return pluxv1.FlagType_FLAG_TYPE_UNSPECIFIED
}

// flagSpecOf reads a flag spec from the wire; an unspecified type stays
// empty, which an update takes as "unchanged" and a create refuses.
func flagSpecOf(m *pluxv1.FlagSpec) experiment.FlagSpec {
	spec := experiment.FlagSpec{
		Type: flagTypes[m.GetType()], Description: m.GetDescription(),
		Default: json.RawMessage(m.GetDefaultJson()),
	}
	if s := m.GetSchemaJson(); s != "" {
		spec.Schema = json.RawMessage(s)
	}
	for _, v := range m.GetSegments() {
		spec.Segments = append(spec.Segments, experiment.SegmentValue{Segment: v.GetSegment(), Value: json.RawMessage(v.GetValueJson())})
	}
	return spec
}

func flagProto(f experiment.Flag) *pluxv1.Flag {
	out := &pluxv1.Flag{
		Id: f.ID, AppId: f.AppID, EnvironmentId: f.EnvironmentID, Key: f.Key, Type: protoFlagType(f.Type),
		Description: f.Description, SchemaJson: string(f.Schema), DefaultJson: string(f.Default),
		CreatedBy: actorProto(f.CreatedBy), UpdatedBy: actorProto(f.UpdatedBy), CreatedAt: ts(f.CreatedAt), UpdatedAt: ts(f.UpdatedAt),
	}
	for _, s := range f.Segments {
		out.Segments = append(out.Segments, &pluxv1.FlagSegmentValue{Segment: s.Segment, ValueJson: s.Value})
	}
	return out
}

func flagRefOf(app, env, key string) experiment.FlagRef {
	return experiment.FlagRef{AppID: app, EnvironmentID: env, Key: key}
}

// CreateFlag makes a flag (ABT-006).
func (s Flags) CreateFlag(ctx context.Context, req *connect.Request[pluxv1.CreateFlagRequest]) (*connect.Response[pluxv1.FlagResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.FlagResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		spec := flagSpecOf(m.GetSpec())
		if spec.Type == "" {
			return nil, plxerr.New(plxerr.UnknownFlagType, "a flag's type is bool, number, string or json")
		}
		f, err := s.h.Experiments.CreateFlag(ctx, p, flagRefOf(m.GetAppId(), m.GetEnvironmentId(), m.GetKey()), spec)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.FlagResponse{Flag: flagProto(f)}, nil
	})
}

// UpdateFlag replaces a flag's content; its type cannot change.
func (s Flags) UpdateFlag(ctx context.Context, req *connect.Request[pluxv1.UpdateFlagRequest]) (*connect.Response[pluxv1.FlagResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.FlagResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		f, err := s.h.Experiments.UpdateFlag(ctx, p, flagRefOf(m.GetAppId(), m.GetEnvironmentId(), m.GetKey()), flagSpecOf(m.GetSpec()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.FlagResponse{Flag: flagProto(f)}, nil
	})
}

// DeleteFlag removes a flag.
func (s Flags) DeleteFlag(ctx context.Context, req *connect.Request[pluxv1.DeleteFlagRequest]) (*connect.Response[pluxv1.DeleteFlagResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteFlagResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		if err := s.h.Experiments.DeleteFlag(ctx, p, flagRefOf(m.GetAppId(), m.GetEnvironmentId(), m.GetKey())); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteFlagResponse{}, nil
	})
}

// GetFlag returns one flag.
func (s Flags) GetFlag(ctx context.Context, req *connect.Request[pluxv1.GetFlagRequest]) (*connect.Response[pluxv1.FlagResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.FlagResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		f, err := s.h.Experiments.GetFlag(ctx, p, flagRefOf(m.GetAppId(), m.GetEnvironmentId(), m.GetKey()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.FlagResponse{Flag: flagProto(f)}, nil
	})
}

// ListFlags lists an environment's flags.
func (s Flags) ListFlags(ctx context.Context, req *connect.Request[pluxv1.ListFlagsRequest]) (*connect.Response[pluxv1.ListFlagsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListFlagsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		flags, err := s.h.Experiments.ListFlags(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListFlagsResponse{}
		for _, f := range flags {
			out.Flags = append(out.Flags, flagProto(f))
		}
		return out, nil
	})
}
