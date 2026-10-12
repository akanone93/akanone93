// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"regexp"
	"slices"
	"strings"
)

// BindingLocation is where a server-held value goes in the request an
// operation forwards (DAT-091).
type BindingLocation string

// The locations a binding may name.
const (
	// BindJSONPointer addresses a member of a JSON request body (RFC 6901).
	BindJSONPointer BindingLocation = "json_pointer"
	// BindHeader is a request header.
	BindHeader BindingLocation = "header"
	// BindPathParam is a path segment inserted directly after the
	// operation's path prefix, in the order of the bindings.
	BindPathParam BindingLocation = "path_param"
	// BindQueryParam is a query parameter.
	BindQueryParam BindingLocation = "query_param"
)

// Binding binds one server-held, server-bound configuration value into
// the request of an operation (DAT-091). The device never sends, chooses
// or sees the value, and a request that sets the location is refused
// (PLX-6065).
type Binding struct {
	// Key is the configuration value.
	Key      string
	Location BindingLocation
	// Name is the JSON Pointer, the header, the path parameter or the
	// query parameter, by Location.
	Name string
	// Display marks a value of a transaction the user sees before they
	// confirm it (DAT-093).
	Display bool
	// Label is what the confirmation sheet calls a displayed value; empty
	// means Key.
	Label string
}

// DisplayLabel is what the confirmation sheet calls the binding's value.
func (b Binding) DisplayLabel() string {
	if b.Label != "" {
		return b.Label
	}
	return b.Key
}

var (
	configKey  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	pathParam  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	queryParam = regexp.MustCompile(`^[A-Za-z0-9_.\-\[\]]{1,64}$`)
)

// validateBindings checks an operation's bindings against its policy.
func (op Operation) validateBindings(u Upstream) error {
	seen := map[string]bool{}
	var pointers []string
	for _, b := range op.Bindings {
		if !configKey.MatchString(b.Key) {
			return invalid("a binding of operation %q names a configuration value: a letter followed by up to 63 letters, digits and underscores", op.Name)
		}
		if len(b.Label) > 64 {
			return invalid("the label of a binding of operation %q is longer than 64 characters", op.Name)
		}
		if err := op.validateLocation(b, u); err != nil {
			return err
		}
		id := string(b.Location) + ":" + strings.ToLower(b.Name)
		if seen[id] {
			return invalid("operation %q binds the %s %q twice", op.Name, b.Location, b.Name)
		}
		seen[id] = true
		if b.Location == BindJSONPointer {
			pointers = append(pointers, b.Name)
		}
		if b.Display {
			if err := op.validateDisplay(b); err != nil {
				return err
			}
		}
	}
	for i, a := range pointers {
		for _, b := range pointers[i+1:] {
			if a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return invalid("operation %q binds a JSON Pointer inside another bound pointer", op.Name)
			}
		}
	}
	return nil
}

// validateLocation checks the name of a binding for its location.
func (op Operation) validateLocation(b Binding, u Upstream) error {
	switch b.Location {
	case BindJSONPointer:
		if b.Name == "" || !pointerForm.MatchString(b.Name) {
			return invalid("operation %q binds a JSON Pointer that names a member of the body", op.Name)
		}
		allowed := u.Methods
		if len(op.Methods) > 0 {
			allowed = op.Methods
		}
		for _, m := range allowed {
			if !slices.Contains(bodyMethods, m) {
				return invalid("operation %q binds a member of the body and so allows only POST, PUT and PATCH", op.Name)
			}
		}
	case BindHeader:
		if !headerName.MatchString(b.Name) || reservedHeader(b.Name) {
			return invalid("operation %q binds a header it may not: %q", op.Name, b.Name)
		}
	case BindPathParam:
		if !pathParam.MatchString(b.Name) {
			return invalid("operation %q binds a path parameter with a name of letters, digits and underscores", op.Name)
		}
		if op.PathPrefix == "" {
			return invalid("operation %q binds a path parameter and so has a path prefix to insert it after", op.Name)
		}
	case BindQueryParam:
		if !queryParam.MatchString(b.Name) {
			return invalid("operation %q binds a query parameter with a plain name", op.Name)
		}
	default:
		return invalid("operation %q has a binding with no location", op.Name)
	}
	return nil
}

// validateDisplay checks a binding the user sees: it fills a member of the
// transaction's summary, so it must sit where the transaction maps one.
func (op Operation) validateDisplay(b Binding) error {
	t := op.Transaction
	if t == nil {
		return invalid("operation %q displays a bound value but is not a transaction", op.Name)
	}
	if b.Location != BindJSONPointer || summaryMember(*t, b.Name) == "" {
		return invalid("a displayed binding of operation %q sits at a JSON Pointer its transaction maps", op.Name)
	}
	return nil
}

// summaryMember names the member of the transaction summary that a
// pointer of the body stands for; empty when the transaction maps none.
func summaryMember(t Transaction, pointer string) string {
	switch pointer {
	case t.Amount:
		return "amount"
	case t.Currency:
		return "currency"
	case t.Payee:
		return "payee"
	}
	if t.Summary != "" && pointer == t.Summary {
		return "summary"
	}
	return ""
}

// jsonBindings reports whether any binding addresses the body.
func (op Operation) jsonBindings() bool {
	for _, b := range op.Bindings {
		if b.Location == BindJSONPointer {
			return true
		}
	}
	return false
}
