// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"strconv"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// SetConfigValue creates or changes a server-held configuration value
// (DAT-090).
func (s SecurityAdmin) SetConfigValue(ctx context.Context, req *connect.Request[pluxv1.SetConfigValueRequest]) (*connect.Response[pluxv1.SetConfigValueResponse], error) {
	if s.h.Configs == nil {
		return s.UnimplementedSecurityAdminServiceHandler.SetConfigValue(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetConfigValueResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		v := configValueFromProto(m.GetValue())
		v.AppID, v.EnvironmentID = m.GetAppId(), m.GetEnvironmentId()
		stored, err := s.h.Configs.Set(ctx, p, v, m.ExpectedVersion)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetConfigValueResponse{Value: configValueProto(stored)}, nil
	})
}

// GetConfigValue returns one value; a sensitive value's content is never
// returned (DAT-090).
func (s SecurityAdmin) GetConfigValue(ctx context.Context, req *connect.Request[pluxv1.GetConfigValueRequest]) (*connect.Response[pluxv1.GetConfigValueResponse], error) {
	if s.h.Configs == nil {
		return s.UnimplementedSecurityAdminServiceHandler.GetConfigValue(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.GetConfigValueResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		v, err := s.h.Configs.Get(ctx, p, m.GetAppId(), m.GetEnvironmentId(), m.GetKey())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetConfigValueResponse{Value: configValueProto(v)}, nil
	})
}

// ListConfigValues lists an environment's values by key.
func (s SecurityAdmin) ListConfigValues(ctx context.Context, req *connect.Request[pluxv1.ListConfigValuesRequest]) (*connect.Response[pluxv1.ListConfigValuesResponse], error) {
	if s.h.Configs == nil {
		return s.UnimplementedSecurityAdminServiceHandler.ListConfigValues(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.ListConfigValuesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := keyQuery(func(m Item) string { return m.(*pluxv1.ConfigValue).GetKey() })
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		scope := req.Msg.GetAppId() + "/" + req.Msg.GetEnvironmentId()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, scope, page)
		if err != nil {
			return nil, err
		}
		values, epoch, err := s.h.Configs.List(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId(), after.Key, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListConfigValuesResponse{Page: &pluxv1.PageResult{}, Epoch: epoch}
		var last storage.Cursor
		for _, v := range values {
			last = storage.Cursor{Key: v.Key}
			if item := configValueProto(v); q.Keep(clauses, item) {
				out.Values = append(out.Values, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, scope, page, len(values), size, last)
		return out, Mask(page.GetReadMask(), out.Values)
	})
}

// DeleteConfigValue removes a value (DAT-090).
func (s SecurityAdmin) DeleteConfigValue(ctx context.Context, req *connect.Request[pluxv1.DeleteConfigValueRequest]) (*connect.Response[pluxv1.DeleteConfigValueResponse], error) {
	if s.h.Configs == nil {
		return s.UnimplementedSecurityAdminServiceHandler.DeleteConfigValue(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteConfigValueResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		if err := s.h.Configs.Delete(ctx, p, m.GetAppId(), m.GetEnvironmentId(), m.GetKey()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteConfigValueResponse{}, nil
	})
}

// ConfigService serves the live configuration to devices (DAT-094).
type ConfigService struct {
	pluxv1connect.UnimplementedConfigServiceHandler
	h *Handlers
}

var _ pluxv1connect.ConfigServiceHandler = ConfigService{}

// ConfigService returns the ConfigService handler.
func (h *Handlers) ConfigService() ConfigService { return ConfigService{h: h} }

// GetLiveConfig returns the live values of the device's environment with
// the version of the snapshot and the epoch (DAT-094, DAT-095). The device
// is the one its DPoP-bound token names; the request carries nothing.
func (s ConfigService) GetLiveConfig(ctx context.Context, _ *connect.Request[pluxv1.GetLiveConfigRequest]) (*connect.Response[pluxv1.GetLiveConfigResponse], error) {
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	snap, err := s.h.LiveConfig.Snapshot(ctx, d.OrganizationID, d.AppID, d.EnvironmentID)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	out := &pluxv1.GetLiveConfigResponse{Version: snap.LiveEpoch, Epoch: snap.Epoch}
	for _, v := range snap.Live() {
		out.Values = append(out.Values, &pluxv1.LiveConfigValue{
			Key: v.Key, Type: configTypeProto(v.Type), Strict: v.Strict, Value: v.Text, Version: v.Version,
		})
	}
	res := connect.NewResponse(out)
	res.Header().Set(HeaderConfigEpoch, strconv.FormatInt(snap.Epoch, 10))
	return res, nil
}

// configValueFromProto converts a value to store; the tier and the type
// that the message does not name stay empty, which validation refuses.
func configValueFromProto(m *pluxv1.ConfigValue) gateway.ConfigValue {
	v := gateway.ConfigValue{Key: m.GetKey(), Sensitive: m.GetSensitive(), Strict: m.GetStrict(), Value: m.GetValue()}
	switch m.GetTier() {
	case pluxv1.ConfigTier_CONFIG_TIER_SERVER_BOUND:
		v.Tier = gateway.TierServerBound
	case pluxv1.ConfigTier_CONFIG_TIER_LIVE:
		v.Tier = gateway.TierLive
	case pluxv1.ConfigTier_CONFIG_TIER_UNSPECIFIED:
	}
	switch m.GetType() {
	case pluxv1.ConfigType_CONFIG_TYPE_STRING:
		v.Type = gateway.TypeString
	case pluxv1.ConfigType_CONFIG_TYPE_INTEGER:
		v.Type = gateway.TypeInteger
	case pluxv1.ConfigType_CONFIG_TYPE_DECIMAL:
		v.Type = gateway.TypeDecimal
	case pluxv1.ConfigType_CONFIG_TYPE_BOOLEAN:
		v.Type = gateway.TypeBoolean
	case pluxv1.ConfigType_CONFIG_TYPE_UNSPECIFIED:
	}
	return v
}

// configTypeProto converts a value type.
func configTypeProto(t gateway.ConfigType) pluxv1.ConfigType {
	switch t {
	case gateway.TypeString:
		return pluxv1.ConfigType_CONFIG_TYPE_STRING
	case gateway.TypeInteger:
		return pluxv1.ConfigType_CONFIG_TYPE_INTEGER
	case gateway.TypeDecimal:
		return pluxv1.ConfigType_CONFIG_TYPE_DECIMAL
	case gateway.TypeBoolean:
		return pluxv1.ConfigType_CONFIG_TYPE_BOOLEAN
	}
	return pluxv1.ConfigType_CONFIG_TYPE_UNSPECIFIED
}

// configValueProto converts a value for a response: a sensitive value
// carries no content (SEC-092).
func configValueProto(v gateway.ConfigValue) *pluxv1.ConfigValue {
	m := &pluxv1.ConfigValue{
		Key: v.Key, Sensitive: v.Sensitive, Strict: v.Strict, Type: configTypeProto(v.Type), Version: v.Version, UpdatedAt: ts(v.UpdatedAt),
	}
	if v.Tier == gateway.TierLive {
		m.Tier = pluxv1.ConfigTier_CONFIG_TIER_LIVE
	} else {
		m.Tier = pluxv1.ConfigTier_CONFIG_TIER_SERVER_BOUND
	}
	if !v.Sensitive {
		m.Value = v.Value
	}
	return m
}
