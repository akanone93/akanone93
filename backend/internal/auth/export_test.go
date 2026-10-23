// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

// CacheLive reports whether the service keeps a permission cache that is
// switched on: it has a change channel and its subscription is live. Tests
// wait for it before they rely on the cache.
func (s *Service) CacheLive() bool {
	if s.bus == nil {
		return false
	}
	live, _ := s.bus.sub.State()
	return live
}
