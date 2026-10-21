// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// waiversKey is the property that waives an accessibility check on a node
// or a page (ADR-0069).
var waiversKey = []byte("a11yWaivers")

// appDocument is the path of the app's own document, which holds its
// security profile.
const appDocument = "app.json"

// waiverChange is an a11yWaivers entry a save adds or changes.
type waiverChange struct {
	// Where names the node by its identifier, or by its pointer when it
	// has none, and the check waived.
	Where string
}

// reviewWaivers checks that a prepared save does not add or change an
// a11yWaivers entry unless the app's profile allows it: under the maximum
// profile the saving principal needs a11y.waive, which requires a second
// factor (PLX-1411, ADR-0069). It returns the entries the save adds or
// changes, which the caller audits. Removing a waiver needs nothing: it
// only makes the checks stricter.
func (s *Service) reviewWaivers(ctx context.Context, q *dbgen.Queries, d draft, p auth.Principal, lim limits.Set, c change) ([]waiverChange, error) {
	if c.delete || !bytes.Contains(c.content, waiversKey) {
		return nil, nil
	}
	depth := int(lim.Get(limits.DocumentJSONDepth))
	parsed, err := jcs.Parse(c.content, depth)
	if err != nil {
		return nil, fmt.Errorf("document: read %s: %w", c.path, err)
	}
	prev, _, err := s.storedTree(ctx, q, d, c.path, depth)
	if err != nil {
		return nil, err
	}
	before := map[string]string{}
	if prev != nil {
		collectWaivers(prev, "", before)
	}
	after := map[string]string{}
	collectWaivers(parsed, "", after)
	var changed []waiverChange
	for where, justification := range after {
		if was, ok := before[where]; !ok || was != justification {
			changed = append(changed, waiverChange{Where: where})
		}
	}
	if len(changed) == 0 {
		return nil, nil
	}
	slices.SortFunc(changed, func(a, b waiverChange) int { return strings.Compare(a.Where, b.Where) })
	profile, err := s.appProfile(ctx, q, d, depth)
	if err != nil {
		return nil, err
	}
	if profile == schema.SecurityProfileMaximum {
		if err := p.AuthorizeApp(auth.A11yWaive, d.app); err != nil {
			return nil, plxerr.Wrap(plxerr.A11yWaiverPermissionRequired, err,
				"%s changes %d accessibility waiver(s) under the maximum profile, which needs %s with a second factor",
				c.path, len(changed), auth.A11yWaive)
		}
	}
	if err := s.requireWaivers(ctx, d, p, c.path, string(profile), changed, after); err != nil {
		return nil, err
	}
	return changed, nil
}

// requireWaivers asks the approval engine whether the waivers a save adds or
// changes may stand (GOV-020, A11Y-008). The approval is about the waivers
// and their justifications, so a changed justification needs a new one. It
// runs before the save writes anything or appends to the audit log
// (approval.Require), which is why the draft carries the transaction.
func (s *Service) requireWaivers(ctx context.Context, d draft, p auth.Principal, path, profile string, changed []waiverChange, justifications map[string]string) error {
	parts := make([][]byte, 0, 2*len(changed))
	for _, c := range changed {
		parts = append(parts, []byte(c.Where), []byte(justifications[c.Where]))
	}
	subject := "waiver:" + path
	if d.plugin != nil {
		subject = "waiver:" + d.plugin.Key + ":" + path
	}
	return approval.Require(ctx, s.o.Approvals, d.tx, approval.Artifact{
		Resource: schema.PolicyResourceWaiver, OrganizationID: p.OrganizationID, AppID: d.app, Action: approval.ActionWaive,
		Subject: subject, Hash: approval.HashOf(parts...), Profile: profile, Requester: p.Actor(),
		Summary: fmt.Sprintf("%d accessibility waiver(s) in %s", len(changed), path),
	})
}

// collectWaivers lists the a11yWaivers entries of a document tree: each
// key names the node (its id, or its pointer without one) and the check,
// and its value is the justification.
func collectWaivers(v any, ptr string, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		node := ptr
		if id, ok := t["id"].(string); ok && id != "" {
			node = id
		}
		if w, ok := t[string(waiversKey)].(map[string]any); ok {
			for check, j := range w {
				text, _ := j.(string)
				out[node+"#"+check] = text
			}
		}
		for k, child := range t {
			if k != string(waiversKey) {
				collectWaivers(child, ptr+"/"+escapePointer(k), out)
			}
		}
	case []any:
		for i, child := range t {
			collectWaivers(child, fmt.Sprintf("%s/%d", ptr, i), out)
		}
	}
}

// escapePointer escapes a property name as a JSON Pointer token.
func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// appProfile reads the security profile of the app document; without one
// it is the standard profile.
func (s *Service) appProfile(ctx context.Context, q *dbgen.Queries, d draft, depth int) (schema.SecurityProfile, error) {
	appDraft := d
	if d.plugin != nil {
		row, err := q.GetDraft(ctx, dbgen.GetDraftParams{AppID: d.row.AppID, PluginID: pgtype.UUID{}})
		if errors.Is(err, pgx.ErrNoRows) {
			return schema.SecurityProfileStandard, nil
		}
		if err != nil {
			return "", fmt.Errorf("document: read the app draft: %w", err)
		}
		appDraft = draft{app: d.app, row: row}
	}
	tree, found, err := s.storedTree(ctx, q, appDraft, appDocument, depth)
	if err != nil || !found {
		return schema.SecurityProfileStandard, err
	}
	if profile, ok := tree["securityProfile"].(string); ok && schema.SecurityProfile(profile).Valid() {
		return schema.SecurityProfile(profile), nil
	}
	return schema.SecurityProfileStandard, nil
}

// auditWaivers records that a save added or changed waivers.
func (s *Service) auditWaivers(ctx context.Context, tx pgx.Tx, p auth.Principal, path string, changed []waiverChange) error {
	if len(changed) == 0 {
		return nil
	}
	where := make([]string, len(changed))
	for i, c := range changed {
		where[i] = c.Where
	}
	return s.recordEntry(ctx, tx, p, audit.Entry{
		Action: audit.A11yWaiverChanged, TargetKind: "document", TargetID: path, Detail: strings.Join(where, ", "),
	})
}
