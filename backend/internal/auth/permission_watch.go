// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// AnnouncePermissionsChange tells every replica that an organisation's
// permissions version has moved on. It is for changes to memberships and app
// access made outside this package, whose transactions already raised the
// version (GOV-007). It is best effort, like every announcement.
func (s *Service) AnnouncePermissionsChange(ctx context.Context, organizationID string) {
	s.announce(ctx, organizationID)
}

// WatchPermissions calls send with the organisation's permissions version,
// first with the current one and then with each newer one as it is
// announced, until the caller goes away, the service is closed or send
// fails (GOV-007). A client that holds an older version refetches its
// effective permissions; nothing polls. A server without a change channel
// cannot watch, and says so rather than staying silent.
func (s *Service) WatchPermissions(ctx context.Context, p Principal, send func(version int64) error) error {
	if s.bus == nil {
		return plxerr.New(plxerr.PreconditionFailed, "this server has no channel for permission change notices")
	}
	// Registered before the current version is read, so that no change
	// between the two goes unseen.
	w, unwatch := s.bus.watch(p.OrganizationID)
	defer unwatch()
	current, err := s.PermissionsVersion(ctx, p.OrganizationID)
	if err != nil {
		return err
	}
	if err := send(current); err != nil {
		return fmt.Errorf("auth.WatchPermissions: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("auth.WatchPermissions: %w", ctx.Err())
		case <-s.bus.stop:
			return nil
		case version := <-w.versions:
			if version == resync {
				if version, err = s.PermissionsVersion(ctx, p.OrganizationID); err != nil {
					return err
				}
			}
			if version <= current {
				continue
			}
			current = version
			if err := send(version); err != nil {
				return fmt.Errorf("auth.WatchPermissions: %w", err)
			}
		}
	}
}
