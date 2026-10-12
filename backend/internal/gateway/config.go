// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"encoding/json"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl/decimal"
)

// ConfigTier says how a configuration value reaches the code that uses
// it (DAT-090).
type ConfigTier string

// The tiers a value may have.
const (
	// TierServerBound values are bound into gateway requests and never
	// leave the server (DAT-091).
	TierServerBound ConfigTier = "server-bound"
	// TierLive values are fetched by devices (DAT-094).
	TierLive ConfigTier = "live"
)

// ConfigType is the type of a configuration value, which is kept as text.
type ConfigType string

// The types a value may have.
const (
	TypeString  ConfigType = "string"
	TypeInteger ConfigType = "integer"
	// TypeDecimal is an exact decimal number, kept and bound as a string.
	TypeDecimal ConfigType = "decimal"
	TypeBoolean ConfigType = "boolean"
)

// ConfigValue is one server-held configuration value of an environment
// (DAT-090).
type ConfigValue struct {
	AppID, EnvironmentID string
	Key                  string
	Tier                 ConfigTier
	Type                 ConfigType
	// Sensitive values are sealed at rest and never returned; only a
	// server-bound value may be sensitive.
	Sensitive bool
	// Strict marks a live value whose change refuses calls made with an
	// older live version (DAT-096).
	Strict bool
	// Value is the text form. It is empty when the value is read back and
	// is sensitive.
	Value     string
	Version   int64
	UpdatedAt time.Time
}

// Validate checks a value a caller wants to store, against the largest
// size the limits allow.
func (v ConfigValue) Validate(maxBytes int64) error {
	if !configKey.MatchString(v.Key) {
		return invalid("the key of a configuration value is a letter followed by up to 63 letters, digits and underscores")
	}
	switch v.Tier {
	case TierServerBound:
		if v.Strict {
			return invalid("strict applies to live values; a server-bound value is always current")
		}
	case TierLive:
		if v.Sensitive {
			return invalid("a live value reaches devices, so it cannot be sensitive")
		}
	default:
		return invalid("%q is not a tier: server-bound or live", v.Tier)
	}
	if int64(len(v.Value)) > maxBytes {
		return plxerr.New(plxerr.LimitExceeded, "a configuration value is at most config.valueBytes = %d bytes", maxBytes)
	}
	return checkConfigText(v.Type, v.Value)
}

// checkConfigText checks that text is a valid value of a type.
func checkConfigText(t ConfigType, text string) error {
	if !utf8.ValidString(text) {
		return invalid("a configuration value is valid UTF-8")
	}
	switch t {
	case TypeString:
		return nil
	case TypeInteger:
		if n, err := strconv.ParseInt(text, 10, 64); err != nil || strconv.FormatInt(n, 10) != text {
			return invalid("the value is not a base-10 integer that fits 64 bits, without a sign or zeros in front")
		}
	case TypeDecimal:
		if _, err := decimal.Parse(text); err != nil {
			return invalid("the value is not a decimal number")
		}
	case TypeBoolean:
		if text != "true" && text != "false" {
			return invalid("the value is true or false")
		}
	default:
		return invalid("%q is not a type: string, integer, decimal or boolean", t)
	}
	return nil
}

// jsonValue is the value as a member of a JSON body: an integer is a
// number, a boolean a boolean, and a string or a decimal a string, so that
// no precision is lost.
func jsonValue(t ConfigType, text string) any {
	switch t {
	case TypeInteger:
		return json.Number(text)
	case TypeBoolean:
		return text == "true"
	}
	return text
}
