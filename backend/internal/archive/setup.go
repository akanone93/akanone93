// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// An app's documents travel in its archive; its server-side setup does not:
// environments and their channels, variables, secrets, configuration
// values, gateway upstreams and identity provider, and the functions with
// their schedules and hooks. The archive's manifest names them, and an
// import lists those the new app lacks as still to do (ADR-0102 Revision
// 2026-11-25, build log row 178). The manifest holds names and kinds only:
// no value of a secret, a configuration value or a variable, no URL, no
// identifier and no time.

// SetupItem is one named piece of an app's server-side setup.
type SetupItem struct {
	// Kind is one of the Setup* kinds.
	Kind string `json:"kind"`
	// Environment is the key of the environment the item belongs to.
	Environment string `json:"environment"`
	// Name is the item's name in the environment: a key, an upstream's or
	// a function's name, a schedule's or a hook's function and alias, or
	// for SetupUnread the kind that could not be read.
	Name string `json:"name,omitempty"`
	// Detail is what recreating it needs beyond its name and holds no
	// value: an environment's "production", a configuration value's tier
	// and type, an upstream's authentication kind, a schedule's expression
	// and time zone.
	Detail string `json:"detail,omitempty"`
}

// The kinds of setup item.
const (
	SetupEnvironment      = "environment"
	SetupChannel          = "channel"
	SetupVariable         = "variable"
	SetupSecret           = "secret"
	SetupConfig           = "config"
	SetupUpstream         = "upstream"
	SetupIdentityProvider = "identityProvider"
	SetupFunction         = "function"
	SetupSchedule         = "schedule"
	SetupHook             = "hook"
	// SetupUnread is a kind of setup of an environment the exporter could
	// not read, which an import lists to be checked by hand.
	SetupUnread = "unread"
)

// knownSetupKind reports whether k is a kind of setup item.
func knownSetupKind(k string) bool {
	switch k {
	case SetupEnvironment, SetupChannel, SetupVariable, SetupSecret, SetupConfig, SetupUpstream,
		SetupIdentityProvider, SetupFunction, SetupSchedule, SetupHook, SetupUnread:
		return true
	}
	return false
}

// maxSetupField bounds one field of a setup item read from an archive.
const maxSetupField = 512

// checkSetup refuses an archive's setup that is not of this format: more
// items than the archive may hold entries, an unknown kind, an item
// without its environment, or a field that is too long or holds a
// control character.
func checkSetup(items []SetupItem, maxItems int64) error {
	if int64(len(items)) > maxItems {
		return plxerr.New(plxerr.LimitExceeded, "the archive names %d setup items; at most %d are accepted", len(items), maxItems)
	}
	for _, it := range items {
		if !knownSetupKind(it.Kind) || it.Environment == "" {
			return plxerr.New(plxerr.InvalidFormat, "the archive names a setup item of kind %q without its environment, or of no known kind", it.Kind)
		}
		for _, f := range []string{it.Environment, it.Name, it.Detail} {
			if len(f) > maxSetupField || strings.IndexFunc(f, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
				return plxerr.New(plxerr.InvalidFormat, "a setup item of the archive has a field that is too long or holds a control character")
			}
		}
	}
	return nil
}

// sortSetup orders setup items for a deterministic manifest, without
// repeats.
func sortSetup(items []SetupItem) []SetupItem {
	out := slices.Clone(items)
	slices.SortFunc(out, func(a, b SetupItem) int {
		return cmp.Or(strings.Compare(a.Environment, b.Environment), strings.Compare(a.Kind, b.Kind),
			strings.Compare(a.Name, b.Name), strings.Compare(a.Detail, b.Detail))
	})
	return slices.Compact(out)
}

// SetupToDo is the setup of want that have lacks, matched by kind,
// environment and name, in order; every SetupUnread item is to do, as
// nobody knows what it stands for.
func SetupToDo(want, have []SetupItem) []SetupItem {
	type key struct{ kind, env, name string }
	there := map[key]bool{}
	for _, it := range have {
		there[key{it.Kind, it.Environment, it.Name}] = true
	}
	out := []SetupItem{}
	for _, it := range sortSetup(want) {
		if it.Kind == SetupUnread || !there[key{it.Kind, it.Environment, it.Name}] {
			out = append(out, it)
		}
	}
	return out
}

// SetupReader reads an app's setup with the caller's permissions.
type SetupReader interface {
	Setup(ctx context.Context, p auth.Principal, appID string) ([]SetupItem, error)
}

// SetupDeps are the services an app's setup is read from. Tenancy is
// required; a nil Configs, Gateway or Functions is a part the installation
// does not run, and is left out.
type SetupDeps struct {
	Tenancy interface {
		ListEnvironments(ctx context.Context, p auth.Principal, appID string, page tenancy.Page) ([]tenancy.Environment, error)
		ListChannels(ctx context.Context, p auth.Principal, envID string, page tenancy.Page) ([]tenancy.Channel, error)
		ListVariables(ctx context.Context, p auth.Principal, envID string, page tenancy.Page) ([]tenancy.Variable, error)
		ListSecrets(ctx context.Context, p auth.Principal, envID string, page tenancy.Page) ([]tenancy.Secret, error)
	}
	Configs interface {
		List(ctx context.Context, p auth.Principal, appID, environmentID, afterKey string, size int32) ([]gateway.ConfigValue, int64, error)
	}
	Gateway interface {
		List(ctx context.Context, p auth.Principal, appID, environmentID string, after storage.Cursor, size int32) ([]gateway.Upstream, error)
		GetProvider(ctx context.Context, p auth.Principal, appID, environmentID string) (gateway.IdentityProvider, error)
	}
	Functions interface {
		ListFunctions(ctx context.Context, p auth.Principal, appID, environmentID, afterName string, size int32) ([]function.Version, error)
		ListTriggers(ctx context.Context, p auth.Principal, appID string) ([]function.Trigger, error)
		ListHooks(ctx context.Context, p auth.Principal, appID string) ([]function.Hook, error)
	}
}

// NewSetupReader returns the reader of an app's setup over deps.
func NewSetupReader(deps SetupDeps) (SetupReader, error) {
	if deps.Tenancy == nil {
		return nil, errors.New("archive: the setup reader needs tenancy")
	}
	return setupReader{deps}, nil
}

type setupReader struct{ d SetupDeps }

// setupPage is the page size the setup is read in.
const setupPage = 200

// Setup reads the setup of every environment of an app. A kind the caller
// may not read is named as SetupUnread rather than refusing the export.
func (r setupReader) Setup(ctx context.Context, p auth.Principal, appID string) ([]SetupItem, error) {
	envs, err := pages(func(after string) ([]tenancy.Environment, string, error) {
		page, err := r.d.Tenancy.ListEnvironments(ctx, p, appID, tenancy.Page{After: storage.Cursor{Key: after}, Size: setupPage})
		return page, lastOf(page, func(e tenancy.Environment) string { return e.Key }), err
	})
	if err != nil {
		return nil, fmt.Errorf("archive: setup: %w", err)
	}
	var out []SetupItem
	keyOf := map[string]string{}
	for _, env := range envs {
		keyOf[env.ID] = env.Key
		detail := ""
		if env.Production {
			detail = "production"
		}
		out = append(out, SetupItem{Kind: SetupEnvironment, Environment: env.Key, Name: env.Key, Detail: detail})
		items, err := r.environment(ctx, p, appID, env)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	if r.d.Functions != nil {
		items, err := r.schedules(ctx, p, appID, keyOf)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return sortSetup(out), nil
}

// environment reads the setup of one environment.
func (r setupReader) environment(ctx context.Context, p auth.Principal, appID string, env tenancy.Environment) ([]SetupItem, error) {
	var out []SetupItem
	add := func(kind string, items []SetupItem, err error) error {
		if ok, err := readable(err); err != nil {
			return fmt.Errorf("archive: setup %s of %s: %w", kind, env.Key, err)
		} else if !ok {
			out = append(out, SetupItem{Kind: SetupUnread, Environment: env.Key, Name: kind})
			return nil
		}
		out = append(out, items...)
		return nil
	}
	for _, part := range r.parts(ctx, p, appID, env) {
		items, err := part.read()
		if err := add(part.kind, items, err); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// part is one kind of an environment's setup and how it is read.
type part struct {
	kind string
	read func() ([]SetupItem, error)
}

// parts are the kinds of an environment's setup the installation runs.
func (r setupReader) parts(ctx context.Context, p auth.Principal, appID string, env tenancy.Environment) []part {
	named := func(kind string, names []string) []SetupItem {
		out := make([]SetupItem, len(names))
		for i, n := range names {
			out[i] = SetupItem{Kind: kind, Environment: env.Key, Name: n}
		}
		return out
	}
	page := func(after string) tenancy.Page {
		return tenancy.Page{After: storage.Cursor{Key: after}, Size: setupPage}
	}
	out := []part{
		{SetupChannel, func() ([]SetupItem, error) {
			names, err := keys(func(after string) ([]tenancy.Channel, error) {
				return r.d.Tenancy.ListChannels(ctx, p, env.ID, page(after))
			}, channelKey)
			return named(SetupChannel, names), err
		}},
		{SetupVariable, func() ([]SetupItem, error) {
			names, err := keys(func(after string) ([]tenancy.Variable, error) {
				return r.d.Tenancy.ListVariables(ctx, p, env.ID, page(after))
			}, variableKey)
			return named(SetupVariable, names), err
		}},
		{SetupSecret, func() ([]SetupItem, error) {
			names, err := keys(func(after string) ([]tenancy.Secret, error) {
				return r.d.Tenancy.ListSecrets(ctx, p, env.ID, page(after))
			}, secretKey)
			return named(SetupSecret, names), err
		}},
	}
	if r.d.Configs != nil {
		out = append(out, part{SetupConfig, func() ([]SetupItem, error) { return r.configs(ctx, p, appID, env) }})
	}
	if r.d.Gateway != nil {
		out = append(out, part{SetupUpstream, func() ([]SetupItem, error) { return r.upstreams(ctx, p, appID, env) }},
			part{SetupIdentityProvider, func() ([]SetupItem, error) { return r.provider(ctx, p, appID, env) }})
	}
	if r.d.Functions != nil {
		out = append(out, part{SetupFunction, func() ([]SetupItem, error) {
			names, err := keys(func(after string) ([]function.Version, error) {
				return r.d.Functions.ListFunctions(ctx, p, appID, env.ID, after, setupPage)
			}, func(v function.Version) string { return v.Name })
			return named(SetupFunction, names), err
		}})
	}
	return out
}

func channelKey(c tenancy.Channel) string   { return c.Key }
func variableKey(v tenancy.Variable) string { return v.Key }
func secretKey(s tenancy.Secret) string     { return s.Key }

// configs names an environment's configuration values with their tier and
// type, never their values.
func (r setupReader) configs(ctx context.Context, p auth.Principal, appID string, env tenancy.Environment) ([]SetupItem, error) {
	values, err := pages(func(after string) ([]gateway.ConfigValue, string, error) {
		page, _, err := r.d.Configs.List(ctx, p, appID, env.ID, after, setupPage)
		return page, lastOf(page, func(v gateway.ConfigValue) string { return v.Key }), err
	})
	out := make([]SetupItem, len(values))
	for i, v := range values {
		detail := string(v.Tier) + " " + string(v.Type)
		if v.Sensitive {
			detail += " sensitive"
		}
		if v.Strict {
			detail += " strict"
		}
		out[i] = SetupItem{Kind: SetupConfig, Environment: env.Key, Name: v.Key, Detail: detail}
	}
	return out, err
}

// upstreams names an environment's upstreams with their authentication
// kind, never their URLs.
func (r setupReader) upstreams(ctx context.Context, p auth.Principal, appID string, env tenancy.Environment) ([]SetupItem, error) {
	var out []SetupItem
	after := storage.Cursor{}
	for {
		page, err := r.d.Gateway.List(ctx, p, appID, env.ID, after, setupPage)
		if err != nil {
			return nil, err //nolint:wrapcheck // wrapped by the caller
		}
		for _, u := range page {
			out = append(out, SetupItem{Kind: SetupUpstream, Environment: env.Key, Name: u.Name, Detail: string(u.Auth.Kind)})
		}
		if len(page) < setupPage {
			return out, nil
		}
		after = storage.Cursor{ID: env.ID, Key: page[len(page)-1].Name}
	}
}

// provider names an environment's identity provider when it has one.
func (r setupReader) provider(ctx context.Context, p auth.Principal, appID string, env tenancy.Environment) ([]SetupItem, error) {
	_, err := r.d.Gateway.GetProvider(ctx, p, appID, env.ID)
	if code, ok := plxerr.CodeOf(err); ok && code == plxerr.ResourceNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	return []SetupItem{{Kind: SetupIdentityProvider, Environment: env.Key}}, nil
}

// schedules names the app's schedules and hooks by function and alias, a
// schedule with its expression and time zone.
func (r setupReader) schedules(ctx context.Context, p auth.Principal, appID string, keyOf map[string]string) ([]SetupItem, error) {
	var out []SetupItem
	triggers, err := r.d.Functions.ListTriggers(ctx, p, appID)
	if ok, err := readable(err); err != nil {
		return nil, fmt.Errorf("archive: setup schedules: %w", err)
	} else if !ok {
		for _, env := range slices.Sorted(mapValues(keyOf)) {
			out = append(out, SetupItem{Kind: SetupUnread, Environment: env, Name: SetupSchedule}, SetupItem{Kind: SetupUnread, Environment: env, Name: SetupHook})
		}
		return out, nil
	}
	for _, t := range triggers {
		out = append(out, SetupItem{Kind: SetupSchedule, Environment: keyOf[t.EnvironmentID], Name: t.Function + "@" + t.Alias, Detail: strings.TrimSpace(t.Expression + " " + t.Timezone)})
	}
	hooks, err := r.d.Functions.ListHooks(ctx, p, appID)
	if err != nil {
		return nil, fmt.Errorf("archive: setup hooks: %w", err)
	}
	for _, h := range hooks {
		out = append(out, SetupItem{Kind: SetupHook, Environment: keyOf[h.EnvironmentID], Name: h.Function + "@" + h.Alias})
	}
	return out, nil
}

// readable reports whether a read succeeded, false without an error when
// the caller may not read it.
func readable(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if code, ok := plxerr.CodeOf(err); ok && code == plxerr.PermissionDenied {
		return false, nil
	}
	return false, err
}

// pages reads every page of a listing that continues after the key its
// last page ended with.
func pages[T any](read func(after string) ([]T, string, error)) ([]T, error) {
	var out []T
	for after := ""; ; {
		page, last, err := read(after)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < setupPage {
			return out, nil
		}
		after = last
	}
}

// keys reads the names of every page of a listing by key.
func keys[T any](read func(after string) ([]T, error), key func(T) string) ([]string, error) {
	all, err := pages(func(after string) ([]T, string, error) {
		page, err := read(after)
		return page, lastOf(page, key), err
	})
	out := make([]string, len(all))
	for i, x := range all {
		out[i] = key(x)
	}
	return out, err
}

// lastOf is the key of a page's last item, or "".
func lastOf[T any](page []T, key func(T) string) string {
	if len(page) == 0 {
		return ""
	}
	return key(page[len(page)-1])
}

// mapValues lists a map's values.
func mapValues(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}
