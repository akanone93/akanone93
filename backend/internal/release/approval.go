// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

var _ approval.Reviewer = (*Service)(nil)

// defaultChannel is the channel a promotion points when it names none.
const defaultChannel = "production"

// promotionSubject names a promotion to the approval engine: the release and
// the channel of the environment it is promoted to, so that an approval is
// spent on exactly that.
func promotionSubject(sequence int64, environment, channel string) string {
	return fmt.Sprintf("release:%d@%s/%s", sequence, environment, channel)
}

// parsePromotionSubject is promotionSubject read back: the release sequence
// and the channel.
func parsePromotionSubject(subject string) (sequence int64, channel string, err error) {
	rest, ok := strings.CutPrefix(subject, "release:")
	seq, target, found := strings.Cut(rest, "@")
	if !ok || !found {
		return 0, "", fmt.Errorf("release: %q is not a promotion", subject)
	}
	sequence, err = strconv.ParseInt(seq, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("release: %q is not a promotion: %w", subject, err)
	}
	_, channel, _ = strings.Cut(target, "/")
	return sequence, channel, nil
}

// releaseHash is the content a promotion approval is bound to: the release
// and the hash of every bundle in it, in key order (GOV-022).
func releaseHash(row dbgen.Release, versions []dbgen.PluginVersion) string {
	parts := [][]byte{[]byte(storage.ID(row.ID)), []byte(strconv.FormatInt(row.Sequence, 10))}
	sorted := slices.Clone(versions)
	slices.SortFunc(sorted, func(a, b dbgen.PluginVersion) int { return strings.Compare(a.PluginKey, b.PluginKey) })
	for _, v := range sorted {
		parts = append(parts, []byte(v.PluginKey), v.BundleSha256)
	}
	return approval.HashOf(parts...)
}

// requirePromotion asks the approval engine whether a release may be pointed
// at a channel (GOV-020, REL-004). The permission and its step-up stay what
// they were. It runs before the channel is locked and before the audit
// entry (approval.Require).
func (s *Service) requirePromotion(ctx context.Context, tx pgx.Tx, p auth.Principal, appID string, env dbgen.Environment, channelKey string, row dbgen.Release, versions []dbgen.PluginVersion) error {
	if s.o.Approvals == nil {
		return nil
	}
	if channelKey == "" {
		channelKey = defaultChannel
	}
	risk, err := s.releaseRisk(ctx, tx, p.OrganizationID, appID, env, channelKey, row, versions)
	if err != nil {
		return err
	}
	art := approval.Artifact{
		Resource: schema.PolicyResourceRelease, OrganizationID: p.OrganizationID, AppID: appID,
		Environment: approval.Environment{ID: storage.ID(env.ID), Key: env.Key, Production: env.Production},
		Action:      approval.ActionPromote, Subject: promotionSubject(row.Sequence, env.Key, channelKey),
		Hash: releaseHash(row, versions), Risk: risk, Requester: p.Actor(),
		Summary: fmt.Sprintf("release %d is promoted to %s/%s", row.Sequence, env.Key, channelKey),
	}
	if row.CreatedByKind == auth.KindUser {
		art.Author = row.CreatedByID
	}
	return approval.Require(ctx, s.o.Approvals, tx, art)
}

// releaseRisk computes the risk attributes of promoting a release from what
// it changes in the channel (GOV-020): the documents that differ from the
// release the channel holds now, read against the compiled project.
func (s *Service) releaseRisk(ctx context.Context, tx pgx.Tx, organizationID, appID string, env dbgen.Environment, channelKey string, row dbgen.Release, versions []dbgen.PluginVersion) ([]schema.PolicyRisk, error) {
	res, _, err := s.compileSet(ctx, tx, organizationID, appID, env.ID, versions)
	if err != nil {
		return nil, err
	}
	if res.Project == nil {
		return nil, nil
	}
	changes, err := s.changesInChannel(ctx, tx, appID, env, channelKey, row.Sequence)
	if err != nil {
		return nil, err
	}
	pinned, err := s.pinnedChanges(ctx, dbgen.New(tx), env, channelKey, row)
	if err != nil {
		return nil, err
	}
	return riskAttributes(res, changes, pinned), nil
}

// changesInChannel lists the documents a release changes against the release
// its channel holds now.
func (s *Service) changesInChannel(ctx context.Context, tx pgx.Tx, appID string, env dbgen.Environment, channelKey string, sequence int64) ([]ChangelogEntry, error) {
	from, err := channelSequence(ctx, dbgen.New(tx), env, channelKey)
	if err != nil {
		return nil, err
	}
	entries, _, err := s.changelogIn(ctx, tx, appID, sequence, from)
	return entries, err
}

// channelSequence is the release a channel points at now, 0 for none.
func channelSequence(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, channelKey string) (int64, error) {
	ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channelKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, failure(err, "channel")
	}
	return ch.ReleaseSequence, nil
}

// pinnedChanges lists the functions whose pinned version differs between the
// release and the release the channel holds now.
func (s *Service) pinnedChanges(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, channelKey string, row dbgen.Release) (map[string]bool, error) {
	ids := []pgtype.UUID{row.ID}
	current, err := channelSequence(ctx, q, env, channelKey)
	if err != nil {
		return nil, err
	}
	var earlier dbgen.Release
	if current > 0 {
		if earlier, err = q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: row.AppID, Sequence: current}); err != nil {
			return nil, failure(err, "release")
		}
		ids = append(ids, earlier.ID)
	}
	rows, err := q.ListReleaseFunctions(ctx, ids)
	if err != nil {
		return nil, failure(err, "release functions")
	}
	before, after := map[string]int32{}, map[string]int32{}
	for _, r := range rows {
		if r.ReleaseID == row.ID {
			after[r.Name] = r.Version
		} else {
			before[r.Name] = r.Version
		}
	}
	changed := map[string]bool{}
	for name, v := range after {
		if was, ok := before[name]; !ok || was != v {
			changed[name] = true
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changed[name] = true
		}
	}
	return changed, nil
}

// riskAttributes decides which risk attributes a release carries: a changed
// page marked secure, a change in a plugin that declares a data source with a
// transaction confirmation, and a changed function version that such a plugin
// grants. The attributes are sorted.
func riskAttributes(res *compiler.Result, changes []ChangelogEntry, changedFunctions map[string]bool) []schema.PolicyRisk {
	var out []schema.PolicyRisk
	if touchesSecurePage(res.Project, changes) {
		out = append(out, schema.PolicyRiskTouchesSecurePage)
	}
	money := moneyPlugins(res.GatewaySources)
	if slices.ContainsFunc(changes, func(c ChangelogEntry) bool { return money[c.PluginKey] }) {
		out = append(out, schema.PolicyRiskTouchesMoneyFlow)
	}
	if grantsChangedFunction(res.Project, money, changedFunctions) {
		out = append(out, schema.PolicyRiskChangesFunctionUsedBy)
	}
	slices.Sort(out)
	return out
}

// touchesSecurePage reports whether an added or changed page is marked
// secure (SEC-090).
func touchesSecurePage(project *schema.Project, changes []ChangelogEntry) bool {
	for _, c := range changes {
		if c.Kind != "page" || c.Change == "removed" {
			continue
		}
		for _, pl := range project.Plugins {
			if pl.Doc == nil || pl.Doc.Key != c.PluginKey {
				continue
			}
			for _, page := range pl.Pages {
				if page.Doc != nil && page.Doc.Key == c.Name && page.Doc.Security != nil && page.Doc.Security.Secure != nil && *page.Doc.Security.Secure {
					return true
				}
			}
		}
	}
	return false
}

// moneyPlugins lists the plugins ("" for the app) that declare a data source
// with a transaction mapping, by key.
func moneyPlugins(sources []compiler.GatewaySource) map[string]bool {
	out := map[string]bool{}
	for _, g := range sources {
		if g.Transaction == nil {
			continue
		}
		if place, ok := schema.PlaceOf(g.File); ok {
			out[place.Plugin] = true
		} else if dir := path.Dir(g.File); dir == "." {
			out[""] = true
		}
	}
	return out
}

// grantsChangedFunction reports whether a plugin of a money flow is granted
// a function whose pinned version changed.
func grantsChangedFunction(project *schema.Project, money map[string]bool, changed map[string]bool) bool {
	if len(changed) == 0 {
		return false
	}
	for _, pl := range project.Plugins {
		if pl.Doc == nil || !money[pl.Doc.Key] || pl.Doc.Capabilities == nil {
			continue
		}
		for _, g := range pl.Doc.Capabilities.Functions {
			if changed[g.Function] {
				return true
			}
		}
	}
	return false
}

// Evidence supplies the review bundle's evidence for a promotion (GOV-023):
// the changes against the channel's current release, the validation and
// accessibility reports, the golden-image differences against the baselines
// the server holds, and the release notes. It implements approval.Reviewer.
func (s *Service) Evidence(ctx context.Context, tx pgx.Tx, r approval.Request) (approval.Evidence, error) {
	sequence, channel, err := parsePromotionSubject(r.Subject)
	if err != nil {
		return approval.Evidence{}, plxerr.Wrap(plxerr.PreconditionFailed, err, "this request is not about a release")
	}
	q := dbgen.New(tx)
	row, versions, err := s.releaseRow(ctx, q, r.AppID, sequence)
	if err != nil {
		return approval.Evidence{}, err
	}
	env, err := q.GetEnvironment(ctx, storage.MustUUID(r.EnvironmentID))
	if err != nil {
		return approval.Evidence{}, failure(err, "environment")
	}
	from, err := channelSequence(ctx, q, env, channel)
	if err != nil {
		return approval.Evidence{}, err
	}
	entries, notes, err := s.changelogIn(ctx, tx, r.AppID, sequence, from)
	if err != nil {
		return approval.Evidence{}, err
	}
	diff, err := json.Marshal(entries)
	if err != nil {
		return approval.Evidence{}, fmt.Errorf("release: encode the changes: %w", err)
	}
	ev := approval.Evidence{Diff: diff, Changelog: changelogLines(entries), Notes: notes}
	if ev.Reports, err = s.reviewReports(ctx, tx, q, r, row, versions); err != nil {
		return approval.Evidence{}, err
	}
	if ev.GoldenDiffs, err = s.goldenDiffs(ctx, tx, r, row, versions); err != nil {
		return approval.Evidence{}, err
	}
	return ev, nil
}

// changelogLines renders changelog entries one to a line.
func changelogLines(entries []ChangelogEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		where := e.Name
		if e.PluginKey != "" {
			where = e.PluginKey + "/" + e.Name
		}
		out[i] = fmt.Sprintf("%s %s %s", e.Change, e.Kind, where)
	}
	return out
}

// reviewReports returns the reports a release carries and the validation
// report of compiling it now: its diagnostics (A11Y-008, I18N-008).
func (s *Service) reviewReports(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, r approval.Request, row dbgen.Release, versions []dbgen.PluginVersion) ([]approval.Report, error) {
	var out []approval.Report
	for _, kind := range []ReportKind{ReportAccessibility, ReportLocalisation} {
		content, err := q.GetReleaseReport(ctx, dbgen.GetReleaseReportParams{ReleaseID: row.ID, Kind: string(kind)})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, failure(err, "release report")
		}
		out = append(out, approval.Report{Kind: string(kind), Content: content})
	}
	res, _, err := s.compileSet(ctx, tx, r.OrganizationID, r.AppID, row.EnvironmentID, versions)
	if err != nil {
		return nil, err
	}
	validation, err := json.Marshal(res.Diagnostics)
	if err != nil {
		return nil, fmt.Errorf("release: encode the diagnostics: %w", err)
	}
	return append(out, approval.Report{Kind: "validation", Content: validation}), nil
}

// goldenDiffs returns the golden-image differences of every bundle of the
// release against the baselines the server holds (TST-003).
func (s *Service) goldenDiffs(ctx context.Context, tx pgx.Tx, r approval.Request, row dbgen.Release, versions []dbgen.PluginVersion) ([]approval.GoldenImage, error) {
	if s.o.Goldens == nil {
		return nil, nil
	}
	appBundle := appBundleHash(row, versions)
	var out []approval.GoldenImage
	for _, v := range versions {
		images, err := s.o.Goldens.ReviewImages(ctx, tx, r.OrganizationID, r.AppID, v.PluginKey, v.BundleSha256, appBundle)
		if err != nil {
			return nil, fmt.Errorf("release: golden differences of %s: %w", nameOf(v), err)
		}
		for _, img := range images {
			out = append(out, approval.GoldenImage{PluginKey: img.PluginKey, CaseID: img.CaseID, Baseline: img.Baseline, Actual: img.Actual})
		}
	}
	return out, nil
}

// appBundleHash is the hash of the release's app bundle.
func appBundleHash(row dbgen.Release, versions []dbgen.PluginVersion) []byte {
	for _, v := range versions {
		if v.ID == row.AppVersionID {
			return v.BundleSha256
		}
	}
	return nil
}
