// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package dsr

import (
	"context"

	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// exportFunc reads at most limit documents of a subject from one store.
type exportFunc func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error)

// eraseFunc deletes a subject's rows from one store and returns how many.
type eraseFunc func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error)

// executor is how one store of the map is exported and erased.
type executor struct {
	// store is the Name of the store in the personal-data map.
	store  string
	export exportFunc
	erase  eraseFunc
}

// executors returns the executors of a kind of subject, in the fixed order
// erasure follows. A device's record comes last, after its credentials.
func executors(kind Kind) []executor {
	if kind == KindUser {
		return userExecutors()
	}
	return deviceExecutors()
}

// deviceExecutors are the stores keyed by a device.
func deviceExecutors() []executor {
	return []executor{
		{
			store: "telemetry_events",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportTelemetryEvents(ctx, dbgen.DsrExportTelemetryEventsParams{OrganizationID: s.org, DeviceID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseTelemetryEvents(ctx, dbgen.DsrEraseTelemetryEventsParams{OrganizationID: s.org, DeviceID: s.id})
			},
		},
		{
			store: "analytics_events",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportAnalyticsEvents(ctx, dbgen.DsrExportAnalyticsEventsParams{OrganizationID: s.org, DeviceID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseAnalyticsEvents(ctx, dbgen.DsrEraseAnalyticsEventsParams{OrganizationID: s.org, DeviceID: s.id})
			},
		},
		{
			store: "device_push_tokens",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportDevicePushTokens(ctx, dbgen.DsrExportDevicePushTokensParams{OrganizationID: s.org, DeviceID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrErasePushTokens(ctx, dbgen.DsrErasePushTokensParams{OrganizationID: s.org, DeviceID: s.id})
			},
		},
		{
			store: "device_sca_keys",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportDeviceSCAKeys(ctx, dbgen.DsrExportDeviceSCAKeysParams{OrganizationID: s.org, DeviceID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseSCAKeys(ctx, dbgen.DsrEraseSCAKeysParams{OrganizationID: s.org, DeviceID: s.id})
			},
		},
		{
			store: "device_tokens",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportDeviceTokens(ctx, dbgen.DsrExportDeviceTokensParams{OrganizationID: s.org, DeviceID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseDeviceTokens(ctx, dbgen.DsrEraseDeviceTokensParams{OrganizationID: s.org, DeviceID: s.id})
			},
		},
		{
			store: "device_bundles",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportDeviceBundles(ctx, dbgen.DsrExportDeviceBundlesParams{OrganizationID: s.org, DeviceID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseDeviceBundles(ctx, dbgen.DsrEraseDeviceBundlesParams{OrganizationID: s.org, DeviceID: s.id})
			},
		},
		{
			store: "devices",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportDevice(ctx, dbgen.DsrExportDeviceParams{OrganizationID: s.org, ID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseDevice(ctx, dbgen.DsrEraseDeviceParams{OrganizationID: s.org, ID: s.id})
			},
		},
	}
}

// userExecutors are the stores keyed by a pseudonymous user.
func userExecutors() []executor {
	return []executor{
		{
			store: "notifications",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportNotifications(ctx, dbgen.DsrExportNotificationsParams{OrganizationID: s.org, UserID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseNotifications(ctx, dbgen.DsrEraseNotificationsParams{OrganizationID: s.org, UserID: s.id})
			},
		},
		{
			store: "notification_subscriptions",
			export: func(ctx context.Context, q *dbgen.Queries, s resolved, limit int32) ([][]byte, error) {
				return q.DsrExportNotificationSubscriptions(ctx, dbgen.DsrExportNotificationSubscriptionsParams{OrganizationID: s.org, SubjectID: s.id, RowLimit: limit})
			},
			erase: func(ctx context.Context, q *dbgen.Queries, s resolved) (int64, error) {
				return q.DsrEraseNotificationSubscriptions(ctx, dbgen.DsrEraseNotificationSubscriptionsParams{OrganizationID: s.org, SubjectID: s.id})
			},
		},
	}
}
