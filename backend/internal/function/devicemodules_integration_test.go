// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// fakeLinker links by concatenating the names of its sources, and counts its
// calls.
type fakeLinker struct {
	mu    sync.Mutex
	calls []function.LinkInput
	diags plxerr.Diagnostics
	err   error
	omit  string
}

func (f *fakeLinker) Link(_ context.Context, in function.LinkInput) (function.LinkOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	if f.err != nil {
		return function.LinkOutput{}, f.err
	}
	if f.diags.HasErrors() {
		return function.LinkOutput{Diagnostics: f.diags}, nil
	}
	module, index := []byte("\x00asm linked"), map[string]uint32{}
	for i, s := range in.Sources {
		module = append(module, s.Name...)
		if s.Name != f.omit {
			index[s.Name] = uint32(i) //nolint:gosec // G115: a handful of sources.
		}
	}
	return function.LinkOutput{Module: module, ABIVersion: 1, Index: index}, nil
}

func (f *fakeLinker) linked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// deviceVersion deploys a device-placed function.
func (w *world) deviceVersion(t *testing.T, name string) function.Version {
	t.Helper()
	return w.deployWith(t, "development", name, function.PlacementDevice, `{"name":"`+name+`","placement":"device"}`, "")
}

// linkedDigest is the key of the module cache as FN-051 defines it: the
// SHA-256 of the sorted list of (name, version, module SHA-256).
func linkedDigest(vs ...function.Version) string {
	h := sha256.New()
	h.Write([]byte("plux/device-module/v1\n"))
	for _, v := range vs {
		fmt.Fprintf(h, "%s\x00%d\x00%x\n", v.Name, v.Version, v.ModuleSHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func refs(vs ...function.Version) []compiler.DeviceFunctionRef {
	out := make([]compiler.DeviceFunctionRef, 0, len(vs))
	for _, v := range vs {
		out = append(out, compiler.DeviceFunctionRef{Name: v.Name, Version: uint32(v.Version)}) //nolint:gosec // G115: small test versions.
	}
	return out
}

// Verifies: FN-051.
// The device functions of a plugin are linked into one module from the
// stored sources of their versions; the same set is linked once, however
// often and by whichever plugin it is asked for, and another set links
// again.
func TestDeviceModulesLinkEachSetOnce_FN_051(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	a, b, c := w.deviceVersion(t, "loans.alpha"), w.deviceVersion(t, "loans.beta"), w.deviceVersion(t, "loans.gamma")
	mods := w.svc.DeviceModules(ctx, w.org, w.app, w.envs["development"])

	first, err := mods.Module("loans", refs(a, b))
	if err != nil || first.ABIVersion != 1 || first.Index["loans.alpha"] != 0 || first.Index["loans.beta"] != 1 || len(first.Wasm) == 0 {
		t.Fatalf("Module = %+v, %v", first, err)
	}
	in := w.linker.calls[0]
	if len(in.Sources) != 2 || in.Sources[0].Name != "loans.alpha" || in.Sources[1].Name != "loans.beta" || in.Sources[0].SourceObjectKey == "" || in.Timeout == 0 {
		t.Errorf("link input = %+v; the sources of the versions, sorted by name", in)
	}
	again, err := mods.Module("accounts", refs(b, a))
	if err != nil || string(again.Wasm) != string(first.Wasm) || w.linker.linked() != 1 {
		t.Errorf("the same set linked %d times: %v", w.linker.linked(), err)
	}
	if _, err := mods.Module("loans", refs(a, b, c)); err != nil || w.linker.linked() != 2 {
		t.Errorf("another set linked %d times in all: %v", w.linker.linked(), err)
	}

	// A newer version of a function is another set.
	a2 := w.deviceVersion(t, "loans.alpha")
	if _, err := mods.Module("loans", refs(a2, b)); err != nil || w.linker.linked() != 3 {
		t.Errorf("a new version linked %d times in all: %v", w.linker.linked(), err)
	}
}

// Verifies: FN-051.
// A module linked before is found in the object store by a process that
// cannot link, and a damaged cache entry is replaced by linking again.
func TestDeviceModulesAreReadFromTheCache_FN_051(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	a := w.deviceVersion(t, "loans.alpha")
	mods := w.svc.DeviceModules(ctx, w.org, w.app, w.envs["development"])
	first, err := mods.Module("loans", refs(a))
	if err != nil {
		t.Fatal(err)
	}

	api := w.service(t, func(o *function.Options) { o.Linker = nil })
	cached, err := api.DeviceModules(ctx, w.org, w.app, w.envs["development"]).Module("loans", refs(a))
	if err != nil || string(cached.Wasm) != string(first.Wasm) || cached.Index["loans.alpha"] != 0 {
		t.Fatalf("cache hit = %+v, %v", cached, err)
	}

	key, err := objects.Key(objects.KindLinked, linkedDigest(a))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := w.store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := w.store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := w.store.Put(ctx, key, data, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.DeviceModules(ctx, w.org, w.app, w.envs["development"]).Module("loans", refs(a)); err == nil {
		t.Error("a process that cannot link used a damaged cache entry")
	}
	if _, err := mods.Module("loans", refs(a)); err != nil || w.linker.linked() != 2 {
		t.Errorf("a damaged entry was not linked again (%d links): %v", w.linker.linked(), err)
	}
	if _, err := api.DeviceModules(ctx, w.org, w.app, w.envs["development"]).Module("loans", refs(a)); err != nil {
		t.Errorf("the module linked again was not stored in place of the damaged entry: %v", err)
	}
}

// Verifies: FN-051, FN-008.
// A set that does not link fails with the builder's diagnostics, a module
// that lacks a function is refused, and a server function is not linked.
func TestDeviceModulesRefuseWhatDoesNotLink_FN_051(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	a := w.deviceVersion(t, "loans.alpha")
	server := w.deployWith(t, "development", "loans.server", function.PlacementServer, `{"name":"loans.server","placement":"server"}`, "")
	mods := w.svc.DeviceModules(ctx, w.org, w.app, w.envs["development"])

	w.linker.diags = plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.FunctionBuildFailed, plxerr.Location{}, "error[E0308]: mismatched types")}
	if _, err := mods.Module("loans", refs(a)); err == nil || !strings.Contains(err.Error(), "error[E0308]") || !strings.Contains(err.Error(), `plugin "loans"`) {
		t.Errorf("a link failure: %v; want the builder's diagnostics", err)
	}
	w.linker.diags = nil
	w.linker.omit = "loans.alpha"
	if _, err := mods.Module("loans", refs(a)); err == nil {
		t.Error("a module that does not export a function was accepted")
	}
	w.linker.omit = ""
	w.linker.err = errors.New("builder unavailable")
	if _, err := mods.Module("loans", refs(a)); err == nil {
		t.Error("a builder failure was not an error")
	}
	w.linker.err = nil
	if _, err := mods.Module("loans", refs(server)); err == nil {
		t.Error("a server function was linked into a device module")
	}
	if _, err := mods.Module("loans", []compiler.DeviceFunctionRef{{Name: "loans.alpha", Version: 9}}); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a version that is not deployed: %v", err)
	}
}

// removeLinked deletes the cached module of the one set the world linked.
func (w *world) removeLinked(t *testing.T) {
	t.Helper()
	versions, err := w.svc.ListFunctions(t.Context(), w.owner, w.app, w.envs["development"], "", 100)
	if err != nil {
		t.Fatal(err)
	}
	key, err := objects.Key(objects.KindLinked, linkedDigest(versions...))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.store.Delete(t.Context(), key); err != nil {
		t.Fatal(err)
	}
}
