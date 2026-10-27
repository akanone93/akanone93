// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

const base = `/o/${orgA}/admin`;
const all = [
  "organization.manage",
  "members.manage",
  "teams.manage",
  "roles.manage",
  "restrictions.manage",
  "sso.manage",
  "tokens.manage",
  "sessions.manage",
  "audit.read",
  "audit.export",
  "compliance.report",
  "retention.manage",
  "limits.manage",
];
const open = (path: string, state: Upstream = {}) =>
  renderApp(`${base}${path}`, { permissions: all, ...state, admin: { calls: [], ...state.admin } });
const calls = (state: Upstream) => state.admin?.calls ?? [];
const type = (label: string | RegExp, value: string) =>
  fireEvent.change(screen.getByRole("textbox", { name: label }), { target: { value } });

describe("STU-116 organisation administration", () => {
  test("O15 renames the organisation and the area's navigation lists every page", async () => {
    const state: Upstream = { admin: { calls: [] } };
    open("", state);
    const box = await screen.findByRole("textbox", { name: "Organisation name" });
    await waitFor(() => expect((box as HTMLInputElement).value).toBe("Alpha Org"));
    expect(screen.getByRole("link", { name: "Audit log" })).toBeTruthy();
    fireEvent.change(box, { target: { value: "Beta" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByText("Saved.");
    expect(calls(state)).toEqual([`updateOrganization:{"id":"${orgA}","name":"Beta"}`]);
  });

  test("a page without its permission shows the forbidden page", async () => {
    renderApp(`${base}/members`, { permissions: [] });
    await screen.findByText("Your role does not include the permission this page needs.");
  });

  test("O16 lists members and shows an invitation once, never again", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        secret: "invite-secret-1",
        roles: [{ key: "editor", name: "Editor", permissions: [] }],
        members: [{ userId: "u2", displayName: "Grace", email: "grace@example.org", role: "editor" }],
      },
    };
    open("/members", state);
    await screen.findByRole("link", { name: "Grace" });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Invite" }).getAttribute("aria-disabled")).not.toBe("true"),
    );
    type("Email address to invite", "new@example.org");
    fireEvent.click(screen.getByRole("button", { name: "Invite" }));
    await screen.findByText("invite-secret-1");
    expect(calls(state)).toEqual(['addMember:{"email":"new@example.org","role":"editor"}']);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(screen.queryByText("invite-secret-1")).toBeNull());
  });

  test("O16 removes a member only after the person types the confirmation", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        members: [{ userId: "u2", displayName: "Grace", email: "grace@example.org", role: "editor" }],
      },
    };
    open("/members", state);
    fireEvent.click(await screen.findByRole("button", { name: "Remove member" }));
    const confirm = await screen.findByRole("textbox");
    expect(calls(state)).toEqual([]);
    fireEvent.change(confirm, { target: { value: "grace@example.org" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(calls(state)).toEqual(['removeMember:{"userId":"u2"}']));
  });

  test("O17 shows a member's grants, restrictions and sessions and adds a grant", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        members: [{ userId: "u2", displayName: "Grace", email: "grace@example.org", role: "editor" }],
        roles: [{ key: "editor", name: "Editor", permissions: [] }],
        grants: [{ id: "g1", userId: "u2", role: "editor", scope: "organization" }],
        restrictions: [
          { id: "r1", subjectKind: "user", subjectId: "u2", patterns: ["release.*"], reason: "contractor" },
        ],
        sessions: [{ id: "s1", userId: "u2", clientKind: "browser" }],
      },
    };
    open("/members/u2", state);
    await screen.findByRole("heading", { name: "Grace" });
    await screen.findByText("release.*");
    expect(screen.getByText("browser")).toBeTruthy();
    expect(screen.getAllByRole("link", { name: "Explain access" }).length).toBe(2);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Add grant" }).getAttribute("aria-disabled")).not.toBe("true"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Add grant" }));
    await waitFor(() =>
      expect(calls(state)).toEqual([
        'createGrant:{"userId":"u2","teamId":"","role":"editor","scope":"organization","appId":""}',
      ]),
    );
  });

  test("O18 and O19 create a team and show a team's members", async () => {
    const state: Upstream = { admin: { calls: [], teams: [{ id: "t1", key: "core", name: "Core" }] } };
    open("/teams", state);
    await screen.findByRole("link", { name: "Core" });
    type("Key", "ops");
    type("Name", "Operations");
    fireEvent.click(screen.getByRole("button", { name: "Create team" }));
    await waitFor(() => expect(calls(state)).toEqual(['createTeam:{"key":"ops","name":"Operations"}']));
    cleanup();
    open("/teams/t1", {
      admin: {
        teams: [{ id: "t1", key: "core", name: "Core" }],
        members: [{ userId: "u3", displayName: "Linus", email: "l@example.org", role: "editor" }],
      },
    });
    await screen.findByRole("heading", { name: "Core" });
    await screen.findByText("l@example.org");
  });

  test("O20 lists predefined and custom roles", async () => {
    open("/roles", {
      admin: {
        roles: [
          { key: "owner", name: "Owner", predefined: true, permissions: [] },
          { key: "qa", name: "QA", permissions: [] },
        ],
      },
    });
    await screen.findByRole("link", { name: "Owner" });
    expect(screen.getByRole("link", { name: "QA" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "New role" })).toBeTruthy();
  });

  test("O21 builds a role from the catalogue tree (area, feature, action) and saves the chosen permissions", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        permissions: [
          { name: "release.publish", description: "Publish" },
          { name: "function.deploy.production", stepUp: true },
          { name: "function.deploy", description: "Deploy" },
        ],
      },
    };
    open("/roles/new", state);
    await screen.findByRole("group", { name: "function" });
    expect(screen.getByRole("group", { name: "release" })).toBeTruthy();
    type("Key", "deployer");
    type("Name", "Deployer");
    fireEvent.click(screen.getByRole("checkbox", { name: "function.deploy" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "release.publish" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(calls(state)).toEqual([
        'createRole:{"key":"deployer","name":"Deployer","permissions":["function.deploy","release.publish"]}',
      ]),
    );
  });

  test("O21 locks a predefined role", async () => {
    open("/roles/owner", {
      admin: {
        permissions: [{ name: "release.publish" }],
        roles: [{ key: "owner", name: "Owner", predefined: true, permissions: ["release.publish"] }],
      },
    });
    await screen.findByText("Predefined roles cannot be changed.");
    expect(screen.getByRole("button", { name: "Save" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("checkbox", { name: "release.publish" }).hasAttribute("disabled")).toBe(true);
  });

  test("O22 adds a restriction with its patterns and removes one", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        restrictions: [
          { id: "r1", subjectKind: "team", subjectId: "t1", patterns: ["function.*"], reason: "no functions" },
        ],
      },
    };
    open("/restrictions", state);
    await screen.findByText("function.*");
    type("Team or person ID", "t9");
    type("Permission patterns (comma-separated)", "release.publish, function.*");
    type("Reason", "contractors");
    fireEvent.click(screen.getByRole("button", { name: "Add restriction" }));
    await waitFor(() =>
      expect(calls(state)).toEqual([
        'createRestriction:{"kind":"organization","subject":"t9","patterns":["release.publish","function.*"],"reason":"contractors"}',
      ]),
    );
    fireEvent.click(screen.getAllByRole("button", { name: "Remove" })[0] as HTMLElement);
    await waitFor(() => expect(calls(state)).toContain('deleteRestriction:{"id":"r1"}'));
  });

  test("O23 explains an allow naming the grant and a deny naming the restriction", async () => {
    open("/access?user=u2&permission=release.publish", {
      admin: { explain: { decision: "allow", allowingGrantIds: ["g1"] } },
    });
    await screen.findByText("Allowed");
    expect(screen.getByText("g1")).toBeTruthy();
    cleanup();
    open("/access?user=u2&permission=release.publish", {
      admin: { explain: { decision: "deny", removingRestrictionIds: ["r7"] } },
    });
    await screen.findByText("Denied");
    expect(screen.getByText("r7")).toBeTruthy();
  });

  test("O24 lists providers and shows a new SCIM secret once", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        secret: "scim-secret-1",
        providers: [{ id: "p1", name: "Okta", kind: "oidc", enabled: true }],
        scimTokens: [{ id: "s1", name: "directory", prefix: "scim_ab" }],
      },
    };
    open("/sso", state);
    await screen.findByText("Okta");
    type("Token name", "hr");
    fireEvent.click(screen.getByRole("button", { name: "Create SCIM token" }));
    await screen.findByText("scim-secret-1");
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(screen.queryByText("scim-secret-1")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Rotate" }));
    await screen.findByText("scim-secret-1");
    expect(calls(state)).toEqual(['createScimToken:{"name":"hr"}', 'rotateScimToken:{"id":"s1"}']);
  });

  test("O25 creates a token (D72), shows the secret once and revokes a token", async () => {
    const state: Upstream = {
      admin: { calls: [], secret: "plx_secret_value", tokens: [{ id: "k1", name: "ci", prefix: "plx_ab" }] },
    };
    open("/tokens", state);
    await screen.findByText("plx_ab");
    type("Token name", "deploy");
    fireEvent.change(screen.getAllByRole("textbox", { name: "Scopes (comma-separated)" })[0] as HTMLElement, {
      target: { value: "release.publish" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create token" }));
    await screen.findByText("plx_secret_value");
    expect(calls(state)).toEqual([
      'createAccessToken:{"name":"deploy","scopes":["release.publish"],"ttlSeconds":"2592000"}',
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(screen.queryByText("plx_secret_value")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(calls(state)).toContain('revokeAccessToken:{"id":"k1"}'));
    expect(document.body.textContent).not.toContain("plx_secret_value");
  });

  test("O25 adds a workload identity", async () => {
    const state: Upstream = { admin: { calls: [] } };
    open("/tokens", state);
    await screen.findByText("Workload identities");
    type("Issuer", "https://ci.example.org");
    type("Audience", "plux");
    type("Subject pattern", "repo:acme/*");
    const scopes = screen.getAllByRole("textbox", { name: "Scopes (comma-separated)" });
    fireEvent.change(scopes[1] as HTMLElement, { target: { value: "release.publish" } });
    fireEvent.click(screen.getByRole("button", { name: "Add workload identity" }));
    await waitFor(() =>
      expect(calls(state)).toEqual([
        'createWorkloadIdentity:{"issuer":"https://ci.example.org","audience":"plux","subject":"repo:acme/*"}',
      ]),
    );
  });

  test("O26 revokes a session", async () => {
    const state: Upstream = { admin: { calls: [], sessions: [{ id: "s1", userId: "u2", clientKind: "browser" }] } };
    open("/sessions", state);
    fireEvent.click(await screen.findByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(calls(state)).toEqual(['revokeSession:{"id":"s1"}']));
  });

  test("O27 sends the action and resource filters to the server and filters the actor in the browser", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        audit: [
          {
            id: "a1",
            action: "role.create",
            actor: "Ada",
            targetKind: "role",
            targetId: "qa",
            atMs: Date.UTC(2026, 0, 5),
          },
          {
            id: "a2",
            action: "role.create",
            actor: "Grace",
            targetKind: "role",
            targetId: "qa",
            atMs: Date.UTC(2026, 0, 6),
          },
        ],
      },
    };
    open("/audit?action=role.create&actor=grace", state);
    await screen.findByText("Grace");
    expect(screen.queryByText("Ada")).toBeNull();
    expect(calls(state)[0]).toBe('listAuditEntries:{"filter":"action = \\"role.create\\""}');
    expect(screen.getByRole("button", { name: "Export CSV" })).toBeTruthy();
  });

  test("O28 creates a compliance report for a period", async () => {
    const state: Upstream = { admin: { calls: [], reports: [{ id: "c1", changes: 4, signed: true }] } };
    open("/compliance", state);
    await screen.findByText("4");
    fireEvent.change(screen.getByLabelText("Period from"), { target: { value: "2026-01-01" } });
    fireEvent.change(screen.getByLabelText("Period to"), { target: { value: "2026-01-31" } });
    fireEvent.click(screen.getByRole("button", { name: "Create report" }));
    await waitFor(() =>
      expect(calls(state)).toEqual(['createComplianceReport:{"from":"1767225600","to":"1769817600"}']),
    );
  });

  test("O29 sets a retention period", async () => {
    const state: Upstream = { admin: { calls: [], retention: [{ dataClass: "telemetry", days: 30 }] } };
    open("/retention", state);
    await screen.findAllByText("telemetry");
    fireEvent.change(screen.getByLabelText("Days to keep"), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(calls(state)).toEqual(['setRetention:{"dataClass":"telemetry","days":60,"immediate":false}']),
    );
  });

  test("O30 shows the licence and disables the upload for anyone but an installation administrator", async () => {
    open("/license", { admin: { license: { state: "valid", licenseId: "lic-1", organization: "Acme" } } });
    await screen.findByText("lic-1");
    expect(screen.getByRole("button", { name: "Upload a licence" }).getAttribute("aria-disabled")).toBe("true");
  });

  test("O30 uploads a licence file as an installation administrator (§4.3 #23)", async () => {
    const state: Upstream = { installationAdmin: true, admin: { calls: [], license: { state: "absent" } } };
    open("/license", state);
    const input = await screen.findByLabelText("Licence file");
    const file = new File([new Uint8Array([1, 2, 3, 4])], "licence.plxlic");
    fireEvent.change(input, { target: { files: [file] } });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Upload a licence" }).getAttribute("aria-disabled")).not.toBe("true"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Upload a licence" }));
    await waitFor(() => expect(calls(state)).toEqual(['installLicense:{"bytes":4}']));
  });

  test("O31 sets a limit override within the registry maximum", async () => {
    const state: Upstream = {
      admin: {
        calls: [],
        limits: [{ key: "asset.fileSize", unit: "bytes", value: 0n, effective: 100n, hardMax: 500n }],
      },
    };
    open("/limits", state);
    const input = await screen.findByRole("textbox", { name: "Override asset.fileSize" });
    fireEvent.change(input, { target: { value: "900" } });
    expect(screen.getByRole("button", { name: "Save" }).getAttribute("aria-disabled")).toBe("true");
    fireEvent.change(input, { target: { value: "300" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(state)).toEqual(['setOrganizationLimit:{"key":"asset.fileSize","value":"300"}']));
  });

  test("a failing read shows the error state", async () => {
    open("/sessions", { admin: { failing: true } });
    await screen.findByText("This could not be loaded.");
  });
});
