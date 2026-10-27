// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package library

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Finding categories of the public scan (STU-081, ADR-0098).
const (
	// CategorySecret is a credential: a private key, an access key or a
	// token in the content.
	CategorySecret = "secret"
	// CategorySensitive is a field tagged sensitive (SCH-012).
	CategorySensitive = "sensitive"
	// CategoryURL is a URL that is not reachable from the public internet
	// over TLS, or that carries credentials.
	CategoryURL = "url"
)

// Finding is one thing the public scan refuses. It names where, never what:
// the value may be the secret itself (SEC-092).
type Finding struct {
	Pointer  string `json:"pointer"`
	Category string `json:"category"`
}

// secretValue matches the shapes of credentials that have a recognisable
// form: private keys, cloud and source-host access keys, payment keys,
// chat tokens, JSON web tokens and bearer headers.
var secretValue = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----` +
	`|AKIA[0-9A-Z]{16}` +
	`|gh[pousr]_[A-Za-z0-9]{30,}` +
	`|sk_live_[0-9a-zA-Z]{16,}` +
	`|xox[abprs]-[0-9A-Za-z-]{10,}` +
	`|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}` +
	`|(?i:bearer)\s+[A-Za-z0-9._~+/=-]{16,}`)

// secretKeys are property names that hold a credential. A property of one of
// these names holding a literal of at least minSecretLength characters is a
// finding; a shorter one, an empty one and an expression are not.
var secretKeys = map[string]bool{
	"password": true, "passwd": true, "secret": true, "clientsecret": true, "apikey": true, "privatekey": true,
	"accesstoken": true, "authtoken": true, "token": true, "authorization": true, "credentials": true,
}

const minSecretLength = 8

var urlPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`)

// privateSuffixes are host name endings that name a private network.
var privateSuffixes = []string{".local", ".internal", ".lan", ".corp", ".intranet", ".home", ".localhost"}

// Scan returns what stops a document from being public, in pointer order. It
// reads a tree as jcs.Parse returns it.
func Scan(tree any) []Finding {
	var out []Finding
	scanValue(tree, "", "", &out)
	slices.SortFunc(out, func(a, b Finding) int {
		if c := strings.Compare(a.Pointer, b.Pointer); c != 0 {
			return c
		}
		return strings.Compare(a.Category, b.Category)
	})
	return slices.Compact(out)
}

func scanValue(v any, pointer, key string, out *[]Finding) {
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t["sensitive"].(bool); ok && s {
			*out = append(*out, Finding{Pointer: pointer, Category: CategorySensitive})
		}
		for k, child := range t {
			scanValue(child, pointer+"/"+escape(k), k, out)
		}
	case []any:
		for i, child := range t {
			scanValue(child, fmt.Sprintf("%s/%d", pointer, i), key, out)
		}
	case string:
		scanString(t, pointer, key, out)
	}
}

func scanString(s, pointer, key string, out *[]Finding) {
	if secretValue.MatchString(s) {
		*out = append(*out, Finding{Pointer: pointer, Category: CategorySecret})
	} else if secretKeys[strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))] &&
		len(s) >= minSecretLength && !strings.HasPrefix(s, "{{") && !strings.HasPrefix(s, "$") {
		*out = append(*out, Finding{Pointer: pointer, Category: CategorySecret})
	}
	for _, raw := range urlPattern.FindAllString(s, -1) {
		if refusedURL(raw) {
			*out = append(*out, Finding{Pointer: pointer, Category: CategoryURL})
			break
		}
	}
}

// refusedURL reports whether a URL may not appear in public content: it is
// not https, carries credentials, or names a host that is not on the public
// internet.
func refusedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return true
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast()
	}
	if !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range privateSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// escape encodes a property name as a JSON pointer token (RFC 6901).
func escape(k string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(k)
}

// scanError is the refusal of a public scan that found something.
func scanError(findings []Finding) error {
	const shown = 10
	var b strings.Builder
	for i, f := range findings {
		if i == shown {
			fmt.Fprintf(&b, ", and %d more", len(findings)-shown)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		pointer := f.Pointer
		if pointer == "" {
			pointer = "/"
		}
		fmt.Fprintf(&b, "%s (%s)", pointer, f.Category)
	}
	list, _ := json.Marshal(findings) //nolint:errchkjson // plain strings
	return (&plxerr.Error{Code: plxerr.LibraryPublicScanFailed, Message: "the content cannot be public: " + b.String()}).WithDetail("findings", string(list))
}
