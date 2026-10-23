// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Cache is what an incremental compilation keeps between calls: the files'
// hashes, the compiler's Memo of the last compilation that had no
// diagnostics, and that compilation's result. It belongs to the caller, is
// bound to the Options it was made with (a different Options needs a new
// Cache), and is not safe for concurrent use.
type Cache struct {
	opts   compiler.Options
	loader *schema.Loader
	// memo, files, groups and byTarget describe the last compilation that
	// had no diagnostics; memo is nil until there is one.
	memo     *compiler.Memo
	files    map[string][sha256.Size]byte
	groups   *groups
	byTarget map[string][]string
	// last is the last compilation that produced bundles: what a device
	// runs, and what Changed and the verdicts are measured against.
	last *Result
	// again is the last compilation that left no Memo, with the digest of
	// the files it compiled, so the same files are not compiled twice.
	again *again
}

// again is a result and the files it was made from.
type again struct {
	digest [sha256.Size]byte
	res    *Result
}

// NewCache returns an empty Cache for compilations with opts.
func NewCache(opts compiler.Options) *Cache { return &Cache{opts: opts} }

// loaderFor returns the loader for the Cache's limits.
func (c *Cache) loaderFor() (*schema.Loader, error) {
	if c.loader == nil {
		l, err := compiler.NewLoader(c.opts)
		if err != nil {
			return nil, fmt.Errorf("incremental.Compile: %w", err)
		}
		c.loader = l
	}
	return c.loader, nil
}

// Input is what to compile.
type Input struct {
	// FS holds the project, laid out as in SCH-006.
	FS fs.FS
}

// Result is the outcome of an incremental compilation.
type Result struct {
	// Result is what the full compiler returns for the project, except
	// that an incremental compilation (Full false) has no diagnostics
	// and none of the whole-project reports: Accessibility, Localisation,
	// StoredState, DeviceUses, Collections and GatewaySources.
	*compiler.Result
	// Full is true when the whole project was compiled; FullReason says
	// why the previous compilation could not be reused.
	Full       bool
	FullReason string
	// Recompiled lists the document files that were recompiled, sorted:
	// the edited pages and the action graphs bound to them, and the pages
	// that refer to them, with theirs (CMP-051). It is nil for a full
	// compilation, which compiles every document.
	Recompiled []string
	// Changed lists the sections that are new or differ from the last
	// compilation that produced bundles, with their bytes; Removed those
	// that are gone.
	Changed []Section
	Removed []SectionRef
	// Pages has the state-compatibility verdict of every page whose
	// section changed (DEV-010).
	Pages []PageVerdict
}

// Compile compiles the project in in.FS, redoing only what changed since the
// last call with this Cache (CMP-051). The bundles are those of
// compiler.Compile. A project with diagnostics gives the full compiler's
// diagnostics and no Changed; the Cache then keeps measuring edits against
// the last good compilation. The error is not nil only when the files cannot
// be read or ctx is done.
func Compile(ctx context.Context, c *Cache, in Input) (*Result, error) {
	if c == nil || in.FS == nil {
		return nil, errors.New("incremental.Compile: a Cache and a file system are required")
	}
	snap, err := takeSnapshot(ctx, in.FS, c.files)
	if err != nil {
		return nil, err
	}
	if c.again != nil && c.again.digest == snap.digest {
		return c.again.res, nil
	}
	if res := c.unchanged(snap); res != nil {
		return res, nil
	}
	res, reason, err := c.incremental(ctx, snap)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return c.full(ctx, in.FS, snap, reason)
	}
	return res, nil
}

// unchanged returns the last result when no file changed since the last
// good compilation.
func (c *Cache) unchanged(snap *snapshot) *Result {
	if c.memo == nil {
		return nil
	}
	if changed, _ := snap.diff(c.files); len(changed) > 0 {
		return nil
	}
	return c.repeat()
}

// repeat returns the last result as a result of a call that changed
// nothing.
func (c *Cache) repeat() *Result {
	r := *c.last
	r.Full, r.FullReason, r.Recompiled, r.Changed, r.Removed, r.Pages = false, "", nil, nil, nil, nil
	return &r
}

// incremental recompiles the pages the changed files belong to. It returns
// no result, and the reason, when it cannot.
func (c *Cache) incremental(ctx context.Context, snap *snapshot) (*Result, string, error) {
	if c.memo == nil {
		return nil, "there is no earlier compilation without diagnostics", nil
	}
	changed, structural := snap.diff(c.files)
	if structural {
		return nil, "files were added or removed", nil
	}
	up, reason := c.parseUpdates(snap, changed, c.groups)
	if up == nil {
		return nil, reason, nil
	}
	if len(up.edited) == 0 {
		c.files = snap.hashes
		return c.repeat(), "", nil
	}
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("incremental.Compile: %w", err)
	}
	project := up.patch(c.memo.Project())
	now := groupsOf(project)
	pages, reason := dependants(up.edited, c.groups, now, c.refs())
	if reason != "" {
		return nil, reason, nil
	}
	res, next, ok := compiler.Recompile(c.memo, project, c.opts, pages)
	if !ok {
		return nil, "the compiler could not reuse the earlier compilation for this edit", nil
	}
	r := &Result{Result: res, Recompiled: unionFiles(c.groups.files(pages), now.files(pages))}
	if err := c.fill(r); err != nil {
		return nil, "", err
	}
	c.accept(r, next, snap.hashes, now)
	return r, "", nil
}

// unionFiles merges two sorted file lists.
func unionFiles(a, b []string) []string {
	set := map[string]bool{}
	for _, s := range [][]string{a, b} {
		for _, f := range s {
			set[f] = true
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

// refs returns the files that refer to each identifier in the earlier
// compilation's reference graph.
func (c *Cache) refs() map[string][]string {
	if c.byTarget == nil {
		c.byTarget = refs(c.memo.Graph())
	}
	return c.byTarget
}

// full compiles the whole project.
func (c *Cache) full(ctx context.Context, fsys fs.FS, snap *snapshot, reason string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("incremental.Compile: %w", err)
	}
	res, memo := compiler.CompileMemo(fsys, c.opts)
	r := &Result{Result: res, Full: true, FullReason: reason}
	if res.App == nil {
		c.again = &again{digest: snap.digest, res: r}
		return r, nil
	}
	if err := c.fill(r); err != nil {
		return nil, err
	}
	if memo == nil {
		// Bundles with diagnostics (warnings): usable, but not a base for
		// recompiling, because a recompilation cannot reproduce the
		// diagnostics of the whole project.
		c.last, c.memo, c.files, c.groups, c.byTarget = r, nil, nil, nil, nil
		c.again = &again{digest: snap.digest, res: r}
		return r, nil
	}
	c.accept(r, memo, snap.hashes, groupsOf(memo.Project()))
	return r, nil
}

// fill sets what changed from the last compilation that produced bundles.
func (c *Cache) fill(r *Result) error {
	var prev *compiler.Result
	var prevProject *schema.Project
	if c.last != nil {
		prev, prevProject = c.last.Result, c.last.Project
	}
	changed, removed, err := sectionDiff(prev, r.Result)
	if err != nil {
		return err
	}
	r.Changed, r.Removed = changed, removed
	r.Pages = verdicts(changed, prevProject, r.Project)
	return nil
}

// accept makes a compilation without diagnostics the base of the next one.
func (c *Cache) accept(r *Result, memo *compiler.Memo, files map[string][sha256.Size]byte, g *groups) {
	c.last, c.memo, c.files, c.groups, c.byTarget, c.again = r, memo, files, g, nil, nil
}
