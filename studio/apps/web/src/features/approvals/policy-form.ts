// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** The fields of the policy editor; lists are kept as the text the person types. */
export interface PolicyForm {
  readonly id: string;
  readonly key: string;
  readonly name: string;
  readonly resources: readonly string[];
  readonly environments: string;
  readonly actions: string;
  readonly production: "any" | "only" | "never";
  readonly risk: readonly string[];
  readonly approvers: number;
  readonly roles: string;
  readonly users: string;
  readonly distinctFromAuthor: boolean;
  readonly distinctTeams: boolean;
  readonly stages: "" | "sequential" | "parallel";
}

interface PolicyDocument {
  id?: string;
  key?: string;
  name?: string;
  match?: {
    resources?: string[];
    environments?: string[];
    actions?: string[];
    production?: boolean;
    risk?: string[];
  };
  require?: {
    approvers?: number;
    roles?: string[];
    users?: string[];
    distinctFromAuthor?: boolean;
    distinctTeams?: boolean;
    stages?: "sequential" | "parallel";
  };
}

const decoder = new TextDecoder();
const encoder = new TextEncoder();

/** What the list shows of a stored document. */
export function documentOf(bytes: Uint8Array): { name: string; approvers: number } {
  const doc = JSON.parse(decoder.decode(bytes)) as PolicyDocument;
  return { name: doc.name ?? "", approvers: doc.require?.approvers ?? 0 };
}

export const emptyForm: PolicyForm = {
  id: "",
  key: "",
  name: "",
  resources: [],
  environments: "",
  actions: "",
  production: "any",
  risk: [],
  approvers: 1,
  roles: "",
  users: "",
  distinctFromAuthor: false,
  distinctTeams: false,
  stages: "",
};

const words = (text: string): string[] =>
  text
    .split(",")
    .map((w) => w.trim())
    .filter((w) => w !== "");

const productionOf = (flag: boolean | undefined): PolicyForm["production"] => {
  if (flag === undefined) {
    return "any";
  }
  return flag ? "only" : "never";
};

/** The form for a stored policy document. */
export function formOf(bytes: Uint8Array): PolicyForm {
  const doc = JSON.parse(decoder.decode(bytes)) as PolicyDocument;
  const { match, require } = doc;
  return {
    id: doc.id ?? "",
    key: doc.key ?? "",
    name: doc.name ?? "",
    resources: match?.resources ?? [],
    environments: (match?.environments ?? []).join(", "),
    actions: (match?.actions ?? []).join(", "),
    production: productionOf(match?.production),
    risk: match?.risk ?? [],
    approvers: require?.approvers ?? 1,
    roles: (require?.roles ?? []).join(", "),
    users: (require?.users ?? []).join(", "),
    distinctFromAuthor: require?.distinctFromAuthor ?? false,
    distinctTeams: require?.distinctTeams ?? false,
    stages: require?.stages ?? "",
  };
}

/** The policy document for the form: only what is set, so that an attribute left out matches anything. */
export function documentBytes(form: PolicyForm, id: string): Uint8Array {
  const match: NonNullable<PolicyDocument["match"]> = {};
  const lists = {
    resources: [...form.resources],
    environments: words(form.environments),
    actions: words(form.actions),
    risk: [...form.risk],
  };
  for (const [name, list] of Object.entries(lists)) {
    if (list.length > 0) {
      match[name as keyof typeof lists] = list;
    }
  }
  if (form.production !== "any") {
    match.production = form.production === "only";
  }
  const req: NonNullable<PolicyDocument["require"]> = { approvers: form.approvers };
  const roles = words(form.roles);
  const users = words(form.users);
  if (roles.length > 0) {
    req.roles = roles;
  }
  if (users.length > 0) {
    req.users = users;
  }
  if (form.distinctFromAuthor) {
    req.distinctFromAuthor = true;
  }
  if (form.distinctTeams) {
    req.distinctTeams = true;
  }
  if (form.stages !== "") {
    req.stages = form.stages;
  }
  const doc = { schemaVersion: "1.0.0", kind: "policy", id, key: form.key, name: form.name, match, require: req };
  return encoder.encode(JSON.stringify(doc));
}
