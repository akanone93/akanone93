// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// resolvedBinding is a binding with the value it binds, as of one epoch.
type resolvedBinding struct {
	Binding
	Value StoredValue
}

// setByDevice is the refusal of a request that sets a bound location.
func setByDevice(b Binding) error {
	return plxerr.New(plxerr.BoundLocationSetByDevice, "the request sets the %s %q, which the gateway binds to a server-held value", b.Location, b.Name)
}

// checkDeviceLocations refuses a request whose headers or query already
// set a bound location (DAT-091). The body is checked when it is read.
func checkDeviceLocations(bound []resolvedBinding, header http.Header, rawQuery string) error {
	var query url.Values
	for _, b := range bound {
		switch b.Location {
		case BindHeader:
			for name := range header {
				if strings.EqualFold(name, b.Name) {
					return setByDevice(b.Binding)
				}
			}
		case BindQueryParam:
			if query == nil {
				// An unparsable query is the upstream's to refuse; what
				// parsed so far still shows a bound name.
				query, _ = url.ParseQuery(rawQuery)
			}
			if _, ok := query[b.Name]; ok {
				return setByDevice(b.Binding)
			}
		case BindJSONPointer, BindPathParam:
		}
	}
	return nil
}

// insertPathParams puts the values of the path parameter bindings into a
// target directly after the operation's path prefix, in binding order. A
// value is escaped as one segment, so one that would need a separator or a
// dot segment is refused by the path checks that follow.
func insertPathParams(op Operation, bound []resolvedBinding, target string) string {
	var segments []string
	for _, b := range bound {
		if b.Location == BindPathParam {
			segments = append(segments, url.PathEscape(b.Value.Text))
		}
	}
	if len(segments) == 0 {
		return target
	}
	path, query, hasQuery := strings.Cut(target, "?")
	prefix := strings.TrimSuffix(op.PathPrefix, "/")
	out := prefix + "/" + strings.Join(segments, "/") + path[len(prefix):]
	if hasQuery {
		out += "?" + query
	}
	return out
}

// appendQueryParams adds the values of the query parameter bindings.
func appendQueryParams(rawQuery string, bound []resolvedBinding) string {
	var b strings.Builder
	b.WriteString(rawQuery)
	for _, rb := range bound {
		if rb.Location != BindQueryParam {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(rb.Name))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(rb.Value.Text))
	}
	return b.String()
}

// boundHeaders returns the headers the bindings set.
func boundHeaders(bound []resolvedBinding) http.Header {
	var out http.Header
	for _, b := range bound {
		if b.Location == BindHeader {
			if out == nil {
				out = http.Header{}
			}
			out.Set(b.Name, b.Value.Text)
		}
	}
	return out
}

// injectBody sets the members the JSON Pointer bindings address. The body
// must be strict JSON with an object at its root; an empty body stands for
// an empty object, so that an operation whose body is all server-held
// needs none from the device. A location the body already sets is
// refused (DAT-091). Numbers keep the text they were written with.
func injectBody(body []byte, bound []resolvedBinding) ([]byte, error) {
	var root map[string]any
	if len(bytes.TrimSpace(body)) == 0 {
		root = map[string]any{}
	} else {
		tree, err := jcs.Parse(body, jsonDepth)
		if err != nil {
			return nil, plxerr.New(plxerr.InvalidFormat, "the body of an operation with server-held values is strict JSON without duplicate members")
		}
		var ok bool
		if root, ok = tree.(map[string]any); !ok {
			return nil, plxerr.New(plxerr.InvalidFormat, "the body of an operation with server-held values is a JSON object")
		}
	}
	for _, b := range bound {
		if b.Location != BindJSONPointer {
			continue
		}
		if err := setPointer(root, b); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	writeJSON(&out, root)
	return out.Bytes(), nil
}

// setPointer sets the member a binding addresses, creating the objects on
// the way.
func setPointer(root map[string]any, b resolvedBinding) error {
	tokens := strings.Split(b.Name[1:], "/")
	cur := root
	for i, token := range tokens {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		if i == len(tokens)-1 {
			if _, exists := cur[token]; exists {
				return setByDevice(b.Binding)
			}
			cur[token] = jsonValue(b.Value.Type, b.Value.Text)
			return nil
		}
		next, exists := cur[token]
		if !exists {
			child := map[string]any{}
			cur[token] = child
			cur = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return setByDevice(b.Binding)
		}
		cur = child
	}
	return nil
}

// writeJSON writes a tree parsed by jcs.Parse, with keys sorted and
// numbers as written.
func writeJSON(out *bytes.Buffer, v any) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			writeString(out, k)
			out.WriteByte(':')
			writeJSON(out, x[k])
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				out.WriteByte(',')
			}
			writeJSON(out, e)
		}
		out.WriteByte(']')
	case string:
		writeString(out, x)
	case json.Number:
		out.WriteString(x.String())
	case bool:
		if x {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	default:
		out.WriteString("null")
	}
}

// writeString writes a JSON string without escaping HTML characters.
func writeString(out *bytes.Buffer, s string) {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	// A string always encodes; the encoder appends a newline to drop.
	_ = enc.Encode(s)
	out.Truncate(out.Len() - 1)
}

// configured is what the server-held configuration makes of a call.
type configured struct {
	target   *url.URL
	bound    []resolvedBinding
	epoch    int64
	hasEpoch bool
}

// configure applies the server-held configuration to a call: it learns
// the epoch for the response, refuses a call made with a stale live
// version (DAT-096), and resolves, checks and applies the operation's
// bindings (DAT-091, DAT-092). A call that needs neither reads only the
// epoch, and goes on without it when the cache store cannot be read.
func (g *Gateway) configure(ctx context.Context, d device.Identity, up Upstream, op Operation, r Request, target *url.URL) (configured, error) {
	out := configured{target: target}
	res := g.o.Config
	if res == nil {
		if len(op.Bindings) > 0 {
			return out, plxerr.New(plxerr.ConfigStoreUnavailable, "this server holds no configuration for the operation's bound values")
		}
		return out, nil
	}
	if len(op.Bindings) == 0 && r.ConfigVersion == nil {
		if epoch, err := res.Epoch(ctx, d.OrganizationID, d.AppID, d.EnvironmentID); err == nil {
			out.epoch, out.hasEpoch = epoch, true
		}
		return out, nil
	}
	snap, err := res.Snapshot(ctx, d.OrganizationID, d.AppID, d.EnvironmentID)
	if err != nil {
		return out, err
	}
	out.epoch, out.hasEpoch = snap.Epoch, true
	if r.ConfigVersion != nil && snap.StrictEpoch > *r.ConfigVersion {
		return out, plxerr.New(plxerr.LiveConfigStale, "a strict live configuration value changed since version %d", *r.ConfigVersion)
	}
	for _, b := range op.Bindings {
		v, err := snap.bound(b.Key)
		if err != nil {
			return out, err
		}
		out.bound = append(out.bound, resolvedBinding{Binding: b, Value: v})
	}
	if err := checkDeviceLocations(out.bound, r.Header, target.RawQuery); err != nil {
		return out, err
	}
	if moved := insertPathParams(op, out.bound, r.Target); moved != r.Target {
		moving := r
		moving.Target = moved
		if out.target, err = checkTarget(up, moving); err != nil {
			return out, err
		}
	}
	out.target.RawQuery = appendQueryParams(out.target.RawQuery, out.bound)
	return out, nil
}

// ConfigEpoch returns the environment's configuration epoch for the
// response of a call that was refused before the gateway read it. It
// reports false when there is none to report (DAT-095).
func (g *Gateway) ConfigEpoch(ctx context.Context, d device.Identity) (int64, bool) {
	if g.o.Config == nil {
		return 0, false
	}
	epoch, err := g.o.Config.Epoch(ctx, d.OrganizationID, d.AppID, d.EnvironmentID)
	return epoch, err == nil
}

// displayed returns the text of the values the user confirmed, by key.
func (p *Pending) displayed() map[string]string {
	var out map[string]string
	for _, b := range p.bound {
		if b.Display {
			if out == nil {
				out = map[string]string{}
			}
			out[b.Key] = b.Value.Text
		}
	}
	return out
}
