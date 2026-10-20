// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

// Notifications configures the delivery of notifications and outbound
// webhooks (SRV-062, SRV-063, ADR-0081).
type Notifications struct {
	// AllowInsecure admits plain-HTTP chat and webhook destinations and
	// SMTP without TLS, and lets them reach loopback addresses. It is for
	// development only: a production installation refuses it and every
	// other start with it set logs a warning.
	AllowInsecure bool `json:"allowInsecure"`
}

// validateNotifications refuses the development setting in production.
func (c *Config) validateNotifications(p *problems) {
	if c.Notifications.AllowInsecure && c.Server.Production {
		p.addf("notifications.allowInsecure", "a production installation sends notifications over TLS only; the setting is for development")
	}
}

// notificationWarnings lists the notification settings that are valid but
// unsafe.
func (c *Config) notificationWarnings() []string {
	if c.Notifications.AllowInsecure {
		return []string{"notifications.allowInsecure is set: notifications and webhooks may use plain HTTP and SMTP without TLS, and reach loopback addresses; it is for development only"}
	}
	return nil
}
