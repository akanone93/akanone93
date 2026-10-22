// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// sortedKeys returns the keys of a map in order.
func sortedKeys(m map[string]any) []string { return slices.Sorted(maps.Keys(m)) }

// person is one of the people the walkthrough provisions.
type person struct {
	actor, email, team string
}

// people are the four provisioned people. alice authors releases, bob and
// carol approve from distinct teams, dave is a contractor.
var people = []person{
	{"alice", "alice@walkthrough.example", "payments"},
	{"bob", "bob@walkthrough.example", "compliance"},
	{"carol", "carol@walkthrough.example", "risk"},
	{"dave", "dave@walkthrough.example", "contractors"},
}

// EmailOf returns the email address of a person step 1 provisions, which a
// hook signs them in with.
func EmailOf(actor string) (string, bool) {
	for _, p := range people {
		if p.actor == actor {
			return p.email, true
		}
	}
	return "", false
}

// approverRole is the custom role of step 1 that may decide on approvals and
// nothing else.
const approverRole = "release-approver"

// maintainerRole is the custom role of step 1 that may change the app's
// documents and plugins, which publishing a project needs.
const maintainerRole = "app-maintainer"

// stepSSO is step 1: the sign-in, the provisioning and the restriction.
func (e *Env) stepSSO(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	e.state.started = e.Now()
	e.state.teams, e.state.users = map[string]string{}, map[string]string{}
	if err := e.provisionPeople(ctx, w); err != nil {
		return fmt.Errorf("SCIM provisioning: %w", err)
	}
	if err := e.signInThroughOIDC(ctx, w); err != nil {
		return fmt.Errorf("OIDC sign-in: %w", err)
	}
	if err := e.createRolesAndGrants(ctx, w); err != nil {
		return fmt.Errorf("roles and grants: %w", err)
	}
	if err := e.issueTokens(ctx); err != nil {
		return err
	}
	return e.restrictContractors(ctx, w)
}

// issueTokens gives every person an access token. It runs after the grants,
// so that each token is issued to a person who already holds their roles.
func (e *Env) issueTokens(ctx context.Context) error {
	for _, p := range people {
		token, err := e.Hooks.IssueToken(ctx, p.actor, p.email)
		if err != nil {
			return fmt.Errorf("a token for %s: %w", p.actor, err)
		}
		e.Creds.SetToken(p.actor, token)
	}
	return nil
}

// signInThroughOIDC creates the identity provider and signs alice in through
// it: the server's sign-in URL, the provider's code, the server's completion.
// The answer is a session or, when the person has a second factor, the
// challenge for it (SEC-100).
func (e *Env) signInThroughOIDC(ctx context.Context, w *walk) error {
	created, err := w.rpc(ctx, e.Admin, federationService+"CreateIdentityProvider", map[string]any{"provider": map[string]any{
		"key": "walkthrough-idp", "kind": "oidc", "name": "Walkthrough test provider", "enabled": true, "jitProvisioning": false,
		"oidc": map[string]any{
			"issuer": e.IDP.Issuer(), "clientId": e.IDP.ClientID(), "clientSecret": e.IDP.ClientSecret(),
			"redirectUrl": strings.TrimSuffix(e.IDP.Issuer(), "/") + "/callback",
		},
	}})
	if err != nil {
		return err
	}
	id, ok := findString(created, "id")
	if !ok {
		return errors.New("the server returned no identity provider ID")
	}
	start, err := w.rpc(ctx, e.Public, identityService+"StartOidcLogin", map[string]any{"providerId": id})
	if err != nil {
		return err
	}
	url, _ := start["url"].(string)
	state, _ := start["state"].(string)
	code, err := e.IDP.Authorize(url, "sub-alice", people[0].email)
	if err != nil {
		return err
	}
	done, err := w.rpc(ctx, e.Public, identityService+"CompleteOidcLogin", map[string]any{"state": state, "code": code})
	if err != nil {
		return err
	}
	_, session := done["session"]
	_, challenge := done["challenge"]
	if !session && !challenge {
		return errors.New("the sign-in ended with neither a session nor a second-factor challenge")
	}
	w.run.Logf("alice signed in through %s: session=%v, second factor asked=%v", e.IDP.Issuer(), session, challenge)
	return nil
}

// provisionPeople makes a SCIM token and provisions the people and their
// teams through SCIM, never through the CLI (GOV-004).
func (e *Env) provisionPeople(ctx context.Context, w *walk) error {
	created, err := w.rpc(ctx, e.Admin, federationService+"CreateScimToken", map[string]any{"name": "walkthrough"})
	if err != nil {
		return err
	}
	secret, _ := created["secret"].(string)
	if secret == "" {
		return errors.New("the server returned no SCIM token")
	}
	for _, p := range people {
		user, err := e.scimCreate(ctx, w, secret, "/scim/v2/Users", map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"externalId": "ext-" + p.actor, "userName": p.email, "active": true,
			"name":   map[string]any{"givenName": p.actor, "familyName": "Walkthrough"},
			"emails": []map[string]any{{"value": p.email, "primary": true}},
		})
		if err != nil {
			return err
		}
		group, err := e.scimCreate(ctx, w, secret, "/scim/v2/Groups", map[string]any{
			"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:Group"}, "displayName": p.team,
			"members": []map[string]any{{"value": user}},
		})
		if err != nil {
			return err
		}
		e.state.users[p.actor], e.state.teams[p.team] = user, group
	}
	return nil
}

// scimCreate posts a SCIM resource, records it without the token and returns
// the ID the server gave it.
func (e *Env) scimCreate(ctx context.Context, w *walk, token, path string, body map[string]any) (string, error) {
	w.run.Logf("SCIM POST %s", path)
	res, err := e.SCIM.Post(ctx, token, path, body)
	text := "request " + redactedJSON(body) + "\nresponse " + redactedJSON(res) + "\n"
	if err != nil {
		text += "error " + err.Error() + "\n"
	}
	if rerr := w.record("scim-"+strings.ToLower(filepath.Base(path)), text); rerr != nil {
		return "", rerr
	}
	if err != nil {
		return "", fmt.Errorf("SCIM %s: %w", path, err)
	}
	id, _ := res["id"].(string)
	if id == "" {
		return "", fmt.Errorf("SCIM %s returned no id", path)
	}
	return id, nil
}

// createRolesAndGrants makes the custom role and grants roles to the teams at
// the organisation's scope.
func (e *Env) createRolesAndGrants(ctx context.Context, w *walk) error {
	if _, err := w.plux(ctx, "admin", "roles", "create", "--name", "Release approver",
		"--permissions", "approvals.decide,app.read,plugin.read,organization.read",
		"--description", "May decide on approval requests and read; nothing else", approverRole); err != nil {
		return err
	}
	// Publishing a project writes its app-level documents and creates its
	// plugins, which release-manager does not hold: the payments team, who
	// build the app, also maintain it.
	if _, err := w.plux(ctx, "admin", "roles", "create", "--name", "App maintainer",
		"--permissions", "app.manage,app.read,plugin.edit,plugin.read,organization.read",
		"--description", "May change the app's documents and plugins", maintainerRole); err != nil {
		return err
	}
	grants := []struct{ team, role string }{
		{"payments", "release-manager"},
		{"payments", maintainerRole},
		{"contractors", "release-manager"},
		{"compliance", approverRole},
		{"compliance", "compliance-officer"},
		{"compliance", "auditor"},
		{"risk", approverRole},
	}
	for _, g := range grants {
		if _, err := w.plux(ctx, "admin", "grants", "create", "--role", g.role, "--team", e.state.teams[g.team], "--scope", "organization"); err != nil {
			return err
		}
	}
	return nil
}

// explainsRestriction matches the words a restriction's explanation uses.
var explainsRestriction = regexp.MustCompile(`(?i)restrict`)

// restrictContractors removes production publishing from the contractors team
// and shows that the explanation names the restriction, although the team's
// role grants the permission (GOV-006, GOV-007).
func (e *Env) restrictContractors(ctx context.Context, w *walk) error {
	args := []string{"access", "explain", "--permission", "release.publish", "--user", e.state.users["dave"], "--app", e.App, "--environment", e.ProdEnv, "--json"}
	out, err := w.plux(ctx, "admin", "restrictions", "create", "--patterns", "release.publish,release.promote",
		"--subject-kind", "team", "--subject", e.state.teams["contractors"], "--app", e.App, "--environment", e.ProdEnv,
		"--reason", "contractors never publish to production", "--json")
	if err != nil {
		return err
	}
	id, err := jsonField(out, "id")
	if err != nil {
		return err
	}
	after, err := w.plux(ctx, "admin", args...)
	if err != nil {
		return err
	}
	if !explainsRestriction.MatchString(after) || !strings.Contains(after, id) {
		return fmt.Errorf("`plux access explain` does not name restriction %s: %s", id, strings.TrimSpace(after))
	}
	_, err = w.plux(ctx, "dave", "whoami", "--json")
	return err
}

// approvalRequired and notEligible match the codes of Appendix F.
var (
	approvalRequired = regexp.MustCompile(`PLX-800[12]`)
	notEligible      = regexp.MustCompile(`PLX-8003|cannot approve|author`)
)

// stepApproval is step 2: the release touching the secure page.
func (e *Env) stepApproval(ctx context.Context, run *Run) error {
	w := e.newWalk(run)
	policy := filepath.Join(e.Project, "policies", "secure-transfer-release.policy.json")
	if _, err := w.plux(ctx, "admin", "policies", "put", "--app", e.App, policy); err != nil {
		return err
	}
	if _, err := w.plux(ctx, "alice", "init", "--server", e.Server, "--org", e.Org, "--app", e.App, "-C", e.Project); err != nil {
		return err
	}
	first, err := e.publishRelease(ctx, w, "alice", "secure transfer, first revision")
	if err != nil {
		return err
	}
	request, err := e.openApprovalRequest(ctx, w, first, "")
	if err != nil {
		return err
	}
	if err := w.refused(ctx, "alice", notEligible, "approvals", "approve", "--app", e.App, "--comment", "my own change", request); err != nil {
		return err
	}
	if _, err := w.plux(ctx, "bob", "approvals", "approve", "--app", e.App, "--comment", "compliance reviewed the diff", request); err != nil {
		return err
	}
	if err := w.refused(ctx, "alice", approvalRequired, "release", "promote", "--app", e.App, "--env", e.ProdEnv, first); err != nil {
		return fmt.Errorf("one approval must not be enough: %w", err)
	}
	return e.changeAndApprove(ctx, w, request)
}

// changeAndApprove changes the content after the first approval, shows that
// the approval does not carry to the new content (GOV-022), and approves the
// new request with two approvers from distinct teams.
func (e *Env) changeAndApprove(ctx context.Context, w *walk, stale string) error {
	if err := e.mutateProject("(revised)"); err != nil {
		return err
	}
	second, err := e.publishRelease(ctx, w, "alice", "secure transfer, revised title")
	if err != nil {
		return err
	}
	request, err := e.openApprovalRequest(ctx, w, second, stale)
	if err != nil {
		return fmt.Errorf("a content change must invalidate the approval: %w", err)
	}
	for _, approver := range []string{"bob", "carol"} {
		if _, err := w.plux(ctx, approver, "approvals", "approve", "--app", e.App, "--comment", "approved the revised content", request); err != nil {
			return err
		}
	}
	_, err = w.plux(ctx, "alice", "release", "promote", "--app", e.App, "--env", e.ProdEnv, "--wait", "2m", second)
	return err
}

// publishRelease publishes the project as a release without promoting it and
// returns its sequence.
func (e *Env) publishRelease(ctx context.Context, w *walk, actor, notes string) (string, error) {
	if _, err := w.plux(ctx, actor, "publish", "-C", e.Project, "--app", e.App, "--release", "--notes", notes); err != nil {
		return "", err
	}
	out, err := w.plux(ctx, actor, "release", "list", "--app", e.App, "--json")
	if err != nil {
		return "", err
	}
	return jsonField(out, "sequence")
}

// openApprovalRequest returns the open approval request that was not open
// before, as `plux approvals list` shows it newest first, after a promotion
// of a release was refused for want of approval. skip is a request ID that
// must not be returned.
func (e *Env) openApprovalRequest(ctx context.Context, w *walk, release, skip string) (string, error) {
	if err := w.refused(ctx, "alice", approvalRequired, "release", "promote", "--app", e.App, "--env", e.ProdEnv, release); err != nil {
		return "", err
	}
	out, err := w.plux(ctx, "alice", "approvals", "list", "--app", e.App, "--status", "open", "--json")
	if err != nil {
		return "", err
	}
	v, err := decode(out)
	if err != nil {
		return "", err
	}
	id, ok := findString(v, "id")
	if !ok || id == skip {
		return "", errors.New("no new open approval request after the refused promotion")
	}
	return id, nil
}
