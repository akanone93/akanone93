// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Button, Dialog, Field, Select, Tooltip } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { AreaTabs, PageFrame } from "../../components/page.tsx";
import { useApp } from "../../lib/use-app.tsx";
import { useEnvironment, usePermissions, useScope } from "../security/use-security.ts";
import { type GatewayMessages, gatewayMessages } from "./gateway.messages.ts";

/** The app, the environment of `?env` and whether the person may change its gateway. */
export function useGatewayScope() {
  const { app, env } = useScope();
  const environment = useEnvironment(app, env);
  const can = usePermissions().has("gateway.manage");
  return { app, env, environment, can };
}

/** The pages of the Gateway area as the header's tab strip. */
function GatewayTabs(): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  const { org, app } = useParams({ strict: false }) as { org: string; app: string };
  const base = `/o/${org}/apps/${app}/gateway`;
  return (
    <AreaTabs
      label={m.areaNav}
      tabs={[
        { href: base, label: m.upstreamsTitle, icon: "network", exact: true },
        { href: `${base}/config`, label: m.configTitle, icon: "sliders" },
        { href: `${base}/identity`, label: m.identityTitle, icon: "users" },
      ]}
    />
  );
}

/** The choice of the app environment whose gateway the pages show, kept in `?env`. */
function EnvironmentChoice({ app, env }: { readonly app: string; readonly env: string }): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  const { session } = useApp();
  const navigate = useNavigate();
  const list = useQuery({
    queryKey: ["environments", app],
    queryFn: () => createClient(AppService, session.transport).listEnvironments({ appId: app }),
  });
  return (
    <Field label={m.environment}>
      <Select
        label={m.environment}
        options={(list.data?.environments ?? []).map((e) => ({ value: e.key, label: e.name || e.key }))}
        value={env}
        onValueChange={(v) => void navigate({ to: ".", search: (s: Record<string, unknown>) => ({ ...s, env: v }) })}
      />
    </Field>
  );
}

/** A Gateway page: its heading, the environment it is about, its tabs and its states. */
export function GatewayFrame(props: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly error?: unknown;
  readonly saveError?: unknown;
  readonly children: ReactNode;
}): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  const { app, env } = useScope();
  return (
    <PageFrame
      id={props.id}
      title={props.title}
      caption={`${props.caption} ${m.stepUpNote}`}
      loading={props.loading}
      failed={props.failed}
      error={props.error}
      saveFailed={props.saveError !== undefined}
      saveError={props.saveError}
      labels={m}
      nav={<GatewayTabs />}
      actions={<EnvironmentChoice app={app} env={env} />}
    >
      {props.children}
    </PageFrame>
  );
}

/** A change button, disabled with the reason when the person may not change the gateway. */
export function ChangeButton(props: {
  readonly can: boolean;
  readonly label: string;
  readonly onClick: () => void;
  readonly variant?: "primary" | "secondary" | "ghost" | "danger";
  readonly size?: "sm" | "md";
}): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  return (
    <Tooltip content={props.can ? undefined : m.noPermission} disabledTrigger={!props.can}>
      <Button
        variant={props.variant ?? "primary"}
        size={props.size ?? "md"}
        disabled={!props.can}
        onClick={props.onClick}
      >
        {props.label}
      </Button>
    </Tooltip>
  );
}

/** Asks before something is removed; `onConfirm` removes it. */
export function RemoveDialog(props: {
  readonly name: string | undefined;
  readonly onClose: () => void;
  readonly onConfirm: () => void;
  readonly m: GatewayMessages;
}): ReactElement {
  const { m } = props;
  return (
    <Dialog
      open={props.name !== undefined}
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      size="sm"
      title={m.remove}
      description={m.removeConfirm(props.name ?? "")}
      labels={{ close: m.close }}
    >
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={props.onClose}>
          {m.cancel}
        </Button>
        <Button variant="danger" onClick={props.onConfirm}>
          {m.remove}
        </Button>
      </div>
    </Dialog>
  );
}

/** The lines of a text, trimmed, without empty ones. */
export const linesOf = (text: string): string[] =>
  text
    .split("\n")
    .map((s) => s.trim())
    .filter((s) => s !== "");
