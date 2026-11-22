// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// ErrLinkerBusy is what a Linker returns when the builder is using every
// build slot it may (FN-016): the link is tried again shortly, within the
// link's own time budget, rather than failing the compilation that asked
// for it.
var ErrLinkerBusy = errors.New("the builder is running as many builds as it may")

// Linker links the sources of several device-placed functions into one
// module on the builder role (FN-051, ADR-0063). Implementations are safe
// for concurrent use. Sources that do not link are a LinkOutput with error
// diagnostics and no module; an error is a failure of the builder itself,
// or ErrLinkerBusy.
type Linker interface {
	Link(ctx context.Context, in LinkInput) (LinkOutput, error)
}

// LinkInput is what the linker is given. As with a build, the
// implementation fetches the archives from the object store by their keys.
type LinkInput struct {
	// LinkID names the link in the builder's logs.
	LinkID string
	// Sources are the functions to link, sorted by name.
	Sources []LinkSource
	// Crates are the stored crate archives the sources depend on, each once
	// however many sources name it (ADR-0105).
	Crates []CrateRef
	// Timeout and MemoryBytes are function.buildTimeout and
	// function.buildMemory (FN-010).
	Timeout     time.Duration
	MemoryBytes int64
	// CacheKey is the organisation's ID, as in BuildInput (ADR-0107).
	CacheKey string
}

// LinkSource is the stored source archive of one function.
type LinkSource struct {
	Name            string
	SourceObjectKey string
	SourceSHA256    string
}

// LinkOutput is what the linker returns.
type LinkOutput struct {
	// Module is the linked module; empty when the link failed.
	Module     []byte
	ABIVersion int32
	// Index maps each function name to the index its dispatch export takes.
	Index       map[string]uint32
	Diagnostics plxerr.Diagnostics
}

// linkDomain separates the digests of the module cache from every other
// SHA-256 of the system; its version changes with the cached format.
const linkDomain = "plux/device-module/v1\n"

// deviceModules implements compiler.DeviceModules over the stored sources
// of an app and environment.
type deviceModules struct {
	s *Service
	// ctx is the context of the operation that compiles: the compiler asks
	// synchronously, so the link runs under the caller's deadline.
	ctx                          context.Context //nolint:containedctx // the compiler's interface has no context to pass.
	organizationID, app, environ string
}

var _ compiler.DeviceModules = deviceModules{}

// DeviceModules returns the supplier of the device modules the compiler
// packs into plugin bundles (FN-051). The functions it is asked for are
// linked once for each set of versions: the linked module is kept in the
// object store under the digest of the sorted list of (name, version,
// module SHA-256), and found there by every later compilation of the same
// set, such as the recompilation when a release is created. ctx bounds the
// links.
func (s *Service) DeviceModules(ctx context.Context, organizationID, appID, environmentID string) compiler.DeviceModules {
	return deviceModules{s: s, ctx: ctx, organizationID: organizationID, app: appID, environ: environmentID}
}

// Module implements compiler.DeviceModules. What it returns as an error is
// shown to the person publishing (PLX-7018), so only a PLX-coded error, which
// says what to fix, passes as it is; the failure of the builder, the store or
// the database is logged here with its cause and named in one plain sentence,
// because its text can carry internal names and addresses (as FailBuild
// does for builds).
func (d deviceModules) Module(pluginKey string, functions []compiler.DeviceFunctionRef) (compiler.DeviceModule, error) {
	mod, err := d.s.deviceModule(d.ctx, d.organizationID, d.app, d.environ, functions)
	if err != nil {
		return compiler.DeviceModule{}, d.s.publicLinkError(d.ctx, pluginKey, err)
	}
	return mod, nil
}

// errLinkStillBusy and errLinkFailed are what a person publishing is told
// when a module could not be linked for a reason that is not theirs to fix.
var (
	errLinkStillBusy = errors.New("the builder stayed busy for as long as a link may take; publish again in a few minutes")
	errLinkFailed    = errors.New("the module could not be linked because a part of the installation failed; the server log has the cause")
)

// publicLinkError returns err when it carries a PLX code, and otherwise logs
// it and returns the plain sentence for its kind.
func (s *Service) publicLinkError(ctx context.Context, pluginKey string, err error) error {
	if _, coded := plxerr.CodeOf(err); coded {
		return err // the compiler's finding names the plugin
	}
	if s.o.Log != nil {
		s.o.Log.ErrorContext(ctx, "a device module could not be linked", slog.String("plugin", pluginKey), slog.String("error", err.Error()))
	}
	if errors.Is(err, ErrLinkerBusy) {
		return errLinkStillBusy
	}
	return errLinkFailed
}

// linkMember is one function version of a module with what linking it
// needs.
type linkMember struct {
	name         string
	version      int32
	moduleSHA256 []byte
	source       dbgen.FunctionSource
	crates       []CrateRef
}

// deviceModule returns the module linking the functions: from the cache,
// or linked now and cached.
func (s *Service) deviceModule(ctx context.Context, organizationID, appID, environmentID string, refs []compiler.DeviceFunctionRef) (compiler.DeviceModule, error) {
	members, lim, err := s.linkMembers(ctx, organizationID, appID, environmentID, refs)
	if err != nil {
		return compiler.DeviceModule{}, err
	}
	key, err := objects.Key(objects.KindLinked, linkDigest(members))
	if err != nil {
		return compiler.DeviceModule{}, fmt.Errorf("function: %w", err)
	}
	cached, found, err := s.cachedModule(ctx, key, members)
	if err != nil || found {
		return cached, err
	}
	if s.o.Linker == nil {
		return compiler.DeviceModule{}, errors.New("function: this process cannot link device modules: it has no route to the builder")
	}
	in := linkInput(key, members, lim)
	in.CacheKey = organizationID
	out, err := s.links.do(ctx, key, func() (LinkOutput, error) { return linkPatiently(ctx, s.o.Linker, in, linkRetryFirst) })
	if err != nil {
		return compiler.DeviceModule{}, fmt.Errorf("function: link: %w", err)
	}
	mod, err := linked(out, members)
	if err != nil {
		return compiler.DeviceModule{}, err
	}
	data, err := encodeCached(mod)
	if err != nil {
		return compiler.DeviceModule{}, err
	}
	if _, err := s.o.Objects.Put(ctx, key, data, "application/octet-stream"); err != nil {
		return compiler.DeviceModule{}, fmt.Errorf("function: store a linked module: %w", err)
	}
	return mod, nil
}

// linkMembers reads the versions to link, with their stored sources, and the
// limits of the app.
func (s *Service) linkMembers(ctx context.Context, organizationID, appID, environmentID string, refs []compiler.DeviceFunctionRef) ([]linkMember, limits.Set, error) {
	var (
		members []linkMember
		lim     limits.Set
	)
	err := s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		for _, r := range refs {
			m, err := linkMemberOf(ctx, q, env.ID, r)
			if err != nil {
				return err
			}
			members = append(members, m)
		}
		if lim, err = s.o.Limits(ctx, tx, organizationID, appID); err != nil {
			return fmt.Errorf("function: %w", err)
		}
		return nil
	})
	slices.SortFunc(members, func(a, b linkMember) int { return strings.Compare(a.name, b.name) })
	return members, lim, err
}

// linkMemberOf reads one version and the source its build was made from.
func linkMemberOf(ctx context.Context, q *dbgen.Queries, environment pgtype.UUID, r compiler.DeviceFunctionRef) (linkMember, error) {
	row, err := q.GetFunctionVersion(ctx, dbgen.GetFunctionVersionParams{EnvironmentID: environment, Name: r.Name, Version: int32(min(r.Version, 1<<31-1))}) //nolint:gosec // G115: clamped.
	if errors.Is(err, pgx.ErrNoRows) {
		return linkMember{}, plxerr.New(plxerr.FunctionNotFound, "no version %d of function %s is deployed in this environment", r.Version, r.Name)
	}
	if err != nil {
		return linkMember{}, fmt.Errorf("function: read version %d of %s: %w", r.Version, r.Name, err)
	}
	if Placement(row.Placement) != PlacementDevice {
		return linkMember{}, plxerr.New(plxerr.FunctionModuleInvalid, "version %d of %s is placed on the %s, not on the device", row.Version, row.Name, row.Placement)
	}
	build, err := q.GetFunctionBuild(ctx, row.BuildID)
	if err != nil {
		return linkMember{}, fmt.Errorf("function: read the build of version %d of %s: %w", row.Version, row.Name, err)
	}
	source, err := q.GetFunctionSource(ctx, build.SourceID)
	if err != nil {
		return linkMember{}, fmt.Errorf("function: read the source of version %d of %s: %w", row.Version, row.Name, err)
	}
	crates, err := sourceCrates(ctx, q, source.ID)
	if err != nil {
		return linkMember{}, err
	}
	return linkMember{name: row.Name, version: row.Version, moduleSHA256: row.ModuleSha256, source: source, crates: crates}, nil
}

// linkDigest is the digest the cache is keyed by: the SHA-256 of the sorted
// list of (name, version, module SHA-256), which commits to every source
// the module is linked from.
func linkDigest(members []linkMember) string {
	h := sha256.New()
	h.Write([]byte(linkDomain))
	for _, m := range members {
		fmt.Fprintf(h, "%s\x00%d\x00%x\n", m.name, m.version, m.moduleSHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// linkInput builds the request to the linker. The link needs no more time
// than a build, and the source of every function is already stored.
func linkInput(key string, members []linkMember, lim limits.Set) LinkInput {
	in := LinkInput{
		LinkID:      key,
		Timeout:     time.Duration(lim.Get(limits.FunctionBuildTimeout)) * time.Millisecond,
		MemoryBytes: lim.Get(limits.FunctionBuildMemory),
	}
	seen := map[string]bool{}
	for _, m := range members {
		in.Sources = append(in.Sources, LinkSource{Name: m.name, SourceObjectKey: m.source.ObjectKey, SourceSHA256: hex.EncodeToString(m.source.Sha256)})
		for _, c := range m.crates {
			if !seen[c.SHA256] {
				seen[c.SHA256] = true
				in.Crates = append(in.Crates, c)
			}
		}
	}
	return in
}

// linked judges what the linker returned: a link that fails with
// diagnostics fails the publish with them, and a module must export exactly
// the functions asked for.
func linked(out LinkOutput, members []linkMember) (compiler.DeviceModule, error) {
	if out.Diagnostics.HasErrors() {
		parts := make([]string, 0, len(out.Diagnostics))
		for _, d := range out.Diagnostics {
			parts = append(parts, fmt.Sprintf("%v: %s", d.Code, d.Message))
		}
		// The builder's findings are about the developer's own code: they are shown as they are (PLX-7200).
		return compiler.DeviceModule{}, plxerr.New(plxerr.FunctionBuildFailed, "the device functions do not link: %s", strings.Join(parts, "; "))
	}
	if len(out.Module) == 0 || out.ABIVersion <= 0 {
		return compiler.DeviceModule{}, errors.New("function: the linker returned no module")
	}
	if len(out.Index) != len(members) {
		return compiler.DeviceModule{}, fmt.Errorf("function: the linked module exports %d functions; %d were asked for", len(out.Index), len(members))
	}
	for _, m := range members {
		if _, ok := out.Index[m.name]; !ok {
			return compiler.DeviceModule{}, fmt.Errorf("function: the linked module does not export %s", m.name)
		}
	}
	return compiler.DeviceModule{Wasm: out.Module, ABIVersion: uint32(out.ABIVersion), Index: out.Index}, nil //nolint:gosec // G115: checked positive.
}

// cachedHeader is the JSON header of a cached module.
type cachedHeader struct {
	ABIVersion   uint32            `json:"abiVersion"`
	ModuleSHA256 string            `json:"moduleSha256"`
	Index        map[string]uint32 `json:"index"`
}

// encodeCached lays a module out as it is cached: the length of the JSON
// header as four big-endian bytes, the header, and the module.
func encodeCached(mod compiler.DeviceModule) ([]byte, error) {
	sum := sha256.Sum256(mod.Wasm)
	header, err := json.Marshal(cachedHeader{ABIVersion: mod.ABIVersion, ModuleSHA256: hex.EncodeToString(sum[:]), Index: mod.Index})
	if err != nil {
		return nil, fmt.Errorf("function: encode a linked module: %w", err)
	}
	out := binary.BigEndian.AppendUint32(nil, uint32(len(header))) //nolint:gosec // G115: a header is far below 4 GiB.
	out = append(out, header...)
	return append(out, mod.Wasm...), nil
}

// decodeCached reads what encodeCached wrote and refuses a module whose
// SHA-256 is not the one the header records.
func decodeCached(data []byte, members []linkMember) (compiler.DeviceModule, error) {
	if len(data) < 4 {
		return compiler.DeviceModule{}, errors.New("function: a cached module is truncated")
	}
	n := int(binary.BigEndian.Uint32(data))
	if n > len(data)-4 {
		return compiler.DeviceModule{}, errors.New("function: a cached module is truncated")
	}
	var h cachedHeader
	if err := json.Unmarshal(data[4:4+n], &h); err != nil {
		return compiler.DeviceModule{}, fmt.Errorf("function: read the header of a cached module: %w", err)
	}
	wasm := data[4+n:]
	if sum := sha256.Sum256(wasm); !strings.EqualFold(hex.EncodeToString(sum[:]), h.ModuleSHA256) {
		return compiler.DeviceModule{}, errors.New("function: a cached module is not the one that was linked")
	}
	return linked(LinkOutput{Module: bytes.Clone(wasm), ABIVersion: int32(min(h.ABIVersion, 1<<31-1)), Index: h.Index}, members) //nolint:gosec // G115: clamped.
}

// cachedModule reads the module linked for the key; found is false when it
// was not linked before or the cache entry was unusable and is removed, in
// which case the set is linked again.
func (s *Service) cachedModule(ctx context.Context, key string, members []linkMember) (mod compiler.DeviceModule, found bool, err error) {
	data, _, err := s.o.Objects.Get(ctx, key)
	if errors.Is(err, objects.ErrNotFound) {
		return compiler.DeviceModule{}, false, nil
	}
	if err != nil {
		return compiler.DeviceModule{}, false, fmt.Errorf("function: read a linked module: %w", err)
	}
	mod, err = decodeCached(data, members)
	if err != nil {
		// Storing a key twice is a no-op, so the damaged entry is removed for
		// the module that is linked in its place to be stored.
		if delErr := s.o.Objects.Delete(ctx, key); delErr != nil {
			return compiler.DeviceModule{}, false, fmt.Errorf("function: remove a damaged linked module: %w", delErr)
		}
		return compiler.DeviceModule{}, false, nil
	}
	return mod, true, nil
}

// linkRetryFirst is the first wait before a busy builder is asked again; each
// later wait doubles, up to linkRetryMost.
const (
	linkRetryFirst = 250 * time.Millisecond
	linkRetryMost  = 4 * time.Second
)

// linkPatiently asks the linker, and while the builder is busy asks again
// after a wait that doubles from first, for as long as the link's own budget
// (its timeout) or ctx allows; the budget bounds the waiting only, not a link
// that has started. The publish jobs of one request compile at once and may
// each need a module, so a builder with one slot is busy for most of them
// (build log #185).
func linkPatiently(ctx context.Context, l Linker, in LinkInput, first time.Duration) (LinkOutput, error) {
	deadline := time.Now().Add(in.Timeout)
	wait := first
	for {
		out, err := l.Link(ctx, in)
		if !errors.Is(err, ErrLinkerBusy) {
			return out, err //nolint:wrapcheck // deviceModule wraps it as "function: link"
		}
		if in.Timeout <= 0 || time.Now().Add(wait).After(deadline) {
			return LinkOutput{}, fmt.Errorf("%w for the whole of the link's time budget", err)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return LinkOutput{}, fmt.Errorf("%w: %w", err, ctx.Err())
		case <-timer.C:
		}
		wait = min(wait*2, linkRetryMost)
	}
}

// linkFlights lets the compilations running at once in this process share
// one link of the same set of versions: the first asks the builder, the
// others wait for its answer, and every later one finds the module in the
// object store.
type linkFlights struct {
	mu      sync.Mutex
	pending map[string]*linkFlight
}

// linkFlight is one link in progress and, once done, its answer.
type linkFlight struct {
	done chan struct{}
	out  LinkOutput
	err  error
}

// do runs link for key, or waits for the link of key already running. A
// waiter whose ctx ends stops waiting; the link itself runs on for the
// others.
func (f *linkFlights) do(ctx context.Context, key string, link func() (LinkOutput, error)) (LinkOutput, error) {
	f.mu.Lock()
	if c, ok := f.pending[key]; ok {
		f.mu.Unlock()
		select {
		case <-c.done:
			return c.out, c.err
		case <-ctx.Done():
			return LinkOutput{}, fmt.Errorf("wait for the link in progress: %w", ctx.Err())
		}
	}
	if f.pending == nil {
		f.pending = map[string]*linkFlight{}
	}
	c := &linkFlight{done: make(chan struct{})}
	f.pending[key] = c
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.pending, key)
		f.mu.Unlock()
		close(c.done)
	}()
	c.out, c.err = link()
	return c.out, c.err
}
