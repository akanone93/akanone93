// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// PolicyRecord is one version of a policy document as stored.
type PolicyRecord struct {
	AppID string
	Key   string
	// Version is 1 for the first document of a key.
	Version int32
	// Document is the canonical JSON of the policy document.
	Document []byte
	// Hash is the SHA-256 of Document, in hex: what an approval of a change
	// to this policy binds to (GOV-022).
	Hash      string
	Deleted   bool
	CreatedBy string
	CreatedAt time.Time
}

// ParsePolicy checks a policy document against its schema and the rules the
// schema cannot state (PLX-1700), and returns it with its canonical form.
func (s *Service) ParsePolicy(raw []byte) (schema.PolicyDocument, []byte, error) {
	tree, err := jcs.Parse(raw, maxDepth)
	if err != nil {
		return schema.PolicyDocument{}, nil, plxerr.Wrap(plxerr.PolicyDocumentInvalid, err, "the policy is not valid JSON")
	}
	if diags := s.validator.Validate(schema.KindPolicy, tree, "policy"); diags.HasErrors() {
		return schema.PolicyDocument{}, nil, plxerr.New(plxerr.PolicyDocumentInvalid, "%s: %s", diags[0].Path, diags[0].Message)
	}
	canonical, err := jcs.Marshal(tree)
	if err != nil {
		return schema.PolicyDocument{}, nil, fmt.Errorf("approval: %w", err)
	}
	var doc schema.PolicyDocument
	if err := json.Unmarshal(canonical, &doc); err != nil {
		return schema.PolicyDocument{}, nil, fmt.Errorf("approval: decode a policy: %w", err)
	}
	if msg := semanticProblem(doc); msg != "" {
		return schema.PolicyDocument{}, nil, plxerr.New(plxerr.PolicyDocumentInvalid, "%s", msg)
	}
	return doc, canonical, nil
}

// semanticProblem states what makes a schema-valid policy impossible to
// satisfy or meaningless, or "" when it is sound. The compiler applies the
// same rules to the policy files of a project (PLX-1700).
func semanticProblem(doc schema.PolicyDocument) string {
	req := doc.Require
	switch {
	case len(doc.Match.Risk) > 0 && len(doc.Match.Resources) > 0 &&
		!slices.Contains(doc.Match.Resources, schema.PolicyResourceRelease) &&
		!slices.Contains(doc.Match.Resources, schema.PolicyResourceExperiment):
		return "/match/risk: risk attributes are computed for releases and experiments, which this policy does not match"
	case len(req.Users) > 0 && len(req.Roles) == 0 && req.Approvers > int64(len(req.Users)):
		return fmt.Sprintf("/require/approvers: %d approvals are required, but only %d approvers are named", req.Approvers, len(req.Users))
	case req.DistinctTeams != nil && *req.DistinctTeams && req.Approvers < 2:
		return "/require/distinctTeams: approvals from distinct teams need at least two approvers"
	case req.Stages == schema.PolicyStagesSequential && req.Approvers > 1 && len(req.Roles)+len(req.Users) == 0:
		return "/require/stages: sequential stages follow the order of the roles and approvers listed, and none is listed"
	}
	return ""
}

// needsEnterprise reports whether a policy uses a feature of the enterprise
// edition: stages or the distinct-team rule (plan §2.6).
func needsEnterprise(doc schema.PolicyDocument) bool {
	req := doc.Require
	return req.Stages != "" || (req.DistinctTeams != nil && *req.DistinctTeams)
}

// checkEnvironments refuses a policy that names an environment the app does
// not have.
func checkEnvironments(ctx context.Context, q *dbgen.Queries, appID string, doc schema.PolicyDocument) error {
	if appID == "" {
		return nil
	}
	app, err := scopeUUID(appID)
	if err != nil {
		return err
	}
	for i, key := range doc.Match.Environments {
		if _, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: app, Key: key}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return plxerr.New(plxerr.PolicyDocumentInvalid, "/match/environments/%d: the app has no environment %q", i, key)
			}
			return fmt.Errorf("approval: read an environment: %w", err)
		}
	}
	return nil
}

// recordOf converts a stored policy version. The database keeps the document
// in its own JSON form, so it is canonicalised again here.
func recordOf(row dbgen.ApprovalPolicy) (PolicyRecord, error) {
	canonical, err := jcs.Canonicalize(row.Document, maxDepth)
	if err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: read the policy %q: %w", row.Key, err)
	}
	return PolicyRecord{
		AppID: appOf(row.AppID), Key: row.Key, Version: row.Version, Document: canonical,
		Hash: row.DocumentHash, Deleted: row.Deleted, CreatedBy: row.CreatedBy, CreatedAt: storage.Time(row.CreatedAt),
	}, nil
}

// policyOf decodes a stored policy version into one the matcher can use.
func policyOf(row dbgen.ApprovalPolicy) (Policy, error) {
	var doc schema.PolicyDocument
	if err := json.Unmarshal(row.Document, &doc); err != nil {
		return Policy{}, fmt.Errorf("approval: decode the policy %q: %w", row.Key, err)
	}
	return Policy{Doc: doc, Hash: row.DocumentHash, Version: row.Version}, nil
}

// PutPolicy writes a policy document as the next version of its key
// (GOV-020). Stages and distinct-team rules need the enterprise licence
// (PLX-8600), and a change to a policy is itself guarded by any policy that
// matches resource "policy" with the action ActionChangePolicy.
func (s *Service) PutPolicy(ctx context.Context, p auth.Principal, appID string, raw []byte) (PolicyRecord, error) {
	if err := authorizeScope(p, auth.PoliciesManage, appID); err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: %w", err)
	}
	doc, canonical, err := s.ParsePolicy(raw)
	if err != nil {
		return PolicyRecord{}, err
	}
	if needsEnterprise(doc) {
		if err := edition.Require(ctx, edition.FeatureMultiStageApprovals); err != nil {
			return PolicyRecord{}, fmt.Errorf("approval: stages and distinct teams: %w", err)
		}
	}
	var out PolicyRecord
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		if err := checkEnvironments(ctx, dbgen.New(tx), appID, doc); err != nil {
			return err
		}
		art := policyArtifact(p, appID, doc.Key, "policy "+doc.Key+" is changed", HashOf(canonical))
		if err := s.Require(ctx, tx, art); err != nil {
			return err
		}
		out, err = s.writePolicy(ctx, tx, p, appID, doc.Key, canonical, false)
		return err
	})
	return out, err
}

// policyArtifact describes a change to a policy.
func policyArtifact(p auth.Principal, appID, key, summary, hash string) Artifact {
	return Artifact{
		Resource: schema.PolicyResourcePolicy, OrganizationID: p.OrganizationID, AppID: appID, Action: ActionChangePolicy,
		Subject: "policy:" + key, Hash: hash, Summary: summary, Requester: p.Actor(),
	}
}

// writePolicy adds the next version of a key, or a tombstone for it, and
// audits it.
func (s *Service) writePolicy(ctx context.Context, tx pgx.Tx, p auth.Principal, appID, key string, document []byte, deleted bool) (PolicyRecord, error) {
	q := dbgen.New(tx)
	app, err := scopeUUID(appID)
	if err != nil {
		return PolicyRecord{}, err
	}
	if err := q.LockApprovalPolicies(ctx, storage.ID(app)); err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: lock the policies: %w", err)
	}
	version := int32(1)
	last, err := q.GetLatestApprovalPolicy(ctx, dbgen.GetLatestApprovalPolicyParams{AppID: app, Key: key})
	switch {
	case err == nil:
		version = last.Version + 1
	case errors.Is(err, pgx.ErrNoRows):
		if deleted {
			return PolicyRecord{}, plxerr.New(plxerr.ResourceNotFound, "no such policy")
		}
	default:
		return PolicyRecord{}, fmt.Errorf("approval: read a policy: %w", err)
	}
	return s.insertPolicy(ctx, tx, p.Actor(), p.OrganizationID, app, key, version, document, deleted)
}

// insertPolicy stores a policy version and audits it as actor.
func (s *Service) insertPolicy(ctx context.Context, tx pgx.Tx, actor audit.Actor, organizationID string, app pgtype.UUID, key string, version int32, document []byte, deleted bool) (PolicyRecord, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: %w", err)
	}
	row, err := dbgen.New(tx).InsertApprovalPolicy(ctx, dbgen.InsertApprovalPolicyParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(organizationID), AppID: app, Key: key, Version: version,
		Document: document, DocumentHash: HashOf(document), Deleted: deleted,
		CreatedByKind: actor.Kind, CreatedByID: actor.ID, CreatedBy: actor.Display,
	})
	if err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: store a policy: %w", err)
	}
	action, verb := audit.ApprovalPolicySet, "written"
	if deleted {
		action, verb = audit.ApprovalPolicyDeleted, "withdrawn"
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: organizationID, Actor: actor, Action: action, TargetKind: "app", TargetID: storage.ID(app),
		AfterHash: row.DocumentHash, Detail: fmt.Sprintf("policy %s version %d %s", key, version, verb),
	})
	if err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: %w", err)
	}
	return recordOf(row)
}

// DeletePolicy withdraws a policy by writing a tombstone version; like a
// change, it can be guarded by a policy on resource "policy".
func (s *Service) DeletePolicy(ctx context.Context, p auth.Principal, appID, key string) error {
	if err := authorizeScope(p, auth.PoliciesManage, appID); err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		app, err := scopeUUID(appID)
		if err != nil {
			return err
		}
		last, err := dbgen.New(tx).GetLatestApprovalPolicy(ctx, dbgen.GetLatestApprovalPolicyParams{AppID: app, Key: key})
		if err != nil || last.Deleted {
			return plxerr.New(plxerr.ResourceNotFound, "no such policy")
		}
		rec, err := recordOf(last)
		if err != nil {
			return err
		}
		art := policyArtifact(p, appID, key, "policy "+key+" is withdrawn", HashOf([]byte("deleted"), rec.Document))
		if err := s.Require(ctx, tx, art); err != nil {
			return err
		}
		_, err = s.writePolicy(ctx, tx, p, appID, key, rec.Document, true)
		return err
	})
}

// GetPolicy returns the policy in force for a key.
func (s *Service) GetPolicy(ctx context.Context, p auth.Principal, appID, key string) (PolicyRecord, error) {
	if err := authorizeScope(p, auth.AppRead, appID); err != nil {
		return PolicyRecord{}, fmt.Errorf("approval: %w", err)
	}
	app, err := scopeUUID(appID)
	if err != nil {
		return PolicyRecord{}, err
	}
	var out PolicyRecord
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetLatestApprovalPolicy(ctx, dbgen.GetLatestApprovalPolicyParams{AppID: app, Key: key})
		if err != nil || row.Deleted {
			return plxerr.New(plxerr.ResourceNotFound, "no such policy")
		}
		out, err = recordOf(row)
		return err
	})
	return out, err
}

// ListPolicies returns the policies in force for an app, by key. The first
// call for an app writes the default policies of the security profile of its
// environments that are not yet there (Q16).
func (s *Service) ListPolicies(ctx context.Context, p auth.Principal, appID string) ([]PolicyRecord, error) {
	if err := authorizeScope(p, auth.AppRead, appID); err != nil {
		return nil, fmt.Errorf("approval: %w", err)
	}
	app, err := scopeUUID(appID)
	if err != nil {
		return nil, err
	}
	var out []PolicyRecord
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		profile, err := s.profileOf(ctx, dbgen.New(tx), Artifact{AppID: appID})
		if err != nil {
			return err
		}
		rows, err := s.latestPolicies(ctx, tx, p.OrganizationID, app, profile, appID != "")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.Deleted {
				continue
			}
			rec, err := recordOf(row)
			if err != nil {
				return err
			}
			out = append(out, rec)
		}
		return nil
	})
	return out, err
}

// ExportPolicies returns the policies in force as the files of the Git
// layout, policies/<key>.policy.json, formatted as the layout requires.
func (s *Service) ExportPolicies(ctx context.Context, p auth.Principal, appID string) (map[string][]byte, error) {
	records, err := s.ListPolicies(ctx, p, appID)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(records))
	for _, r := range records {
		formatted, err := jcs.Format(r.Document, maxDepth)
		if err != nil {
			return nil, fmt.Errorf("approval: format the policy %q: %w", r.Key, err)
		}
		files["policies/"+r.Key+".policy.json"] = formatted
	}
	return files, nil
}

// latestPolicies reads the newest version of every key of an app, writing
// the defaults of the profile that are missing first when seeded is true (an
// organisation has none). A default an
// administrator withdrew is a tombstone and counts as present, so it is not
// written again.
func (s *Service) latestPolicies(ctx context.Context, tx pgx.Tx, organizationID string, app pgtype.UUID, profile settings.Profile, seeded bool) ([]dbgen.ApprovalPolicy, error) {
	rows, err := dbgen.New(tx).ListLatestApprovalPolicies(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("approval: list the policies: %w", err)
	}
	if !seeded {
		return rows, nil
	}
	missing, err := s.missingDefaults(rows, profile)
	if err != nil || len(missing) == 0 {
		return rows, err
	}
	if err := s.seed(ctx, organizationID, app, missing); err != nil {
		return nil, err
	}
	rows, err = dbgen.New(tx).ListLatestApprovalPolicies(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("approval: list the policies: %w", err)
	}
	return rows, nil
}

// missingDefaults returns the default policies of a profile that have no row.
func (s *Service) missingDefaults(rows []dbgen.ApprovalPolicy, profile settings.Profile) ([]schema.PolicyDocument, error) {
	defaults, err := DefaultPolicies(profile, s.o.IDs.New)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(defaults, func(d schema.PolicyDocument) bool {
		return slices.ContainsFunc(rows, func(r dbgen.ApprovalPolicy) bool { return r.Key == d.Key })
	}), nil
}

// seed writes default policies in a transaction of its own, so that it is
// committed whatever becomes of the caller's, and audits them as the system.
// It takes the policies' lock, which is why Require runs before a caller
// takes it.
func (s *Service) seed(ctx context.Context, organizationID string, app pgtype.UUID, docs []schema.PolicyDocument) error {
	actor := auth.System(organizationID).Actor()
	return s.beside(ctx, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.LockApprovalPolicies(ctx, storage.ID(app)); err != nil {
			return fmt.Errorf("approval: lock the policies: %w", err)
		}
		for _, d := range docs {
			if _, err := q.GetLatestApprovalPolicy(ctx, dbgen.GetLatestApprovalPolicyParams{AppID: app, Key: d.Key}); err == nil {
				continue
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("approval: read a policy: %w", err)
			}
			raw, err := json.Marshal(d)
			if err != nil {
				return fmt.Errorf("approval: encode a default policy: %w", err)
			}
			canonical, err := jcs.Canonicalize(raw, maxDepth)
			if err != nil {
				return fmt.Errorf("approval: canonicalise a default policy: %w", err)
			}
			if _, err := s.insertPolicy(ctx, tx, actor, organizationID, app, d.Key, 1, canonical, false); err != nil {
				return err
			}
		}
		return nil
	})
}

// policiesIn returns the policies in force for an app as the matcher uses
// them: the app's own, with the defaults of its profile, and the
// organisation's.
func (s *Service) policiesIn(ctx context.Context, tx pgx.Tx, organizationID, appID string, profile settings.Profile) ([]Policy, error) {
	var rows []dbgen.ApprovalPolicy
	scopes := []string{""}
	if appID != "" {
		scopes = []string{appID, ""}
	}
	for _, scope := range scopes {
		uuid, err := scopeUUID(scope)
		if err != nil {
			return nil, err
		}
		got, err := s.latestPolicies(ctx, tx, organizationID, uuid, profile, scope != "")
		if err != nil {
			return nil, err
		}
		rows = append(rows, got...)
	}
	out := make([]Policy, 0, len(rows))
	for _, row := range rows {
		if row.Deleted {
			continue
		}
		pol, err := policyOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, pol)
	}
	return out, nil
}
