// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { libraryMessages } from "./library.messages.ts";
import { saveFile } from "./library-model.ts";
import { useCan, useLibraryClients } from "./use-library.ts";
import { VisibilityDialog } from "./visibility-dialog.tsx";

/** O6: one template's parameters, revisions and visibility (STU-080 to STU-083). */
export function TemplateDetailPage(): ReactElement {
  const m = libraryMessages(useLingui().i18n);
  const { org = "", id = "" } = useParams({ strict: false }) as { org?: string; id?: string };
  const clients = useLibraryClients();
  const canPublish = useCan("library.publish");
  const canManage = useCan("library.manage");
  const [changing, setChanging] = useState(false);
  const detail = useQuery({
    queryKey: ["library", "template", id],
    queryFn: async () => {
      const [versions, template] = await Promise.all([
        clients.templates.listTemplateVersions({ id }),
        clients.templates.getTemplate({ id }),
      ]);
      return { versions, template: template.template };
    },
  });
  const item = detail.data?.versions.item;
  const revisions = [...(detail.data?.versions.versions ?? [])].sort((a, b) => b.version - a.version);
  const parameters = detail.data?.template?.parameters ?? [];
  const exportVersion = async (version: number) => {
    const file = await clients.templates.exportTemplate({ id, version });
    saveFile(file.fileName, file.file);
  };
  return (
    <section className="flex flex-col gap-4" aria-labelledby="template-title">
      <Link to="/o/$org/library" params={{ org }} className="text-caption">
        {m.back}
      </Link>
      {detail.isPending && <p className="text-body text-secondary">{m.loading}</p>}
      {detail.isError && (
        <p role="alert" className="text-body text-danger">
          {m.detailLoadFailed}
        </p>
      )}
      {item !== undefined && (
        <>
          <h1 id="template-title" className="text-screen">
            {item.name}
          </h1>
          <p className="text-body text-secondary">{item.description}</p>
          {(canPublish || canManage) && (
            <div>
              <Button variant="secondary" onClick={() => setChanging(true)}>
                {m.changeVisibility}
              </Button>
            </div>
          )}
          <h2 className="text-body font-medium">{m.parametersHeading}</h2>
          {parameters.length === 0 && <p className="text-body text-secondary">{m.parametersNone}</p>}
          <ul>
            {parameters.map((p) => (
              <li key={p.name} className="text-body">
                {m.parameterRow(p.name, p.type, p.required)}
                {p.description !== "" && <span className="text-caption text-secondary"> {p.description}</span>}
              </li>
            ))}
          </ul>
          <h2 className="text-body font-medium">{m.revisionsHeading}</h2>
          <ul>
            {revisions.map((v) => (
              <li key={v.version} className="flex items-center gap-3 py-1">
                <span className="text-body">{m.versionRow(v.version, v.createdByName)}</span>
                <span className="text-caption text-secondary">{v.note}</span>
                <Button variant="ghost" size="sm" onClick={() => void exportVersion(v.version)}>
                  {m.exportVersion}
                </Button>
              </li>
            ))}
          </ul>
        </>
      )}
      {changing && item !== undefined && (
        <VisibilityDialog
          current={item.visibility}
          canPublish={canPublish}
          apply={async (visibility) => {
            await clients.templates.setTemplateVisibility({ id, visibility });
            await clients.invalidate();
          }}
          onClose={() => setChanging(false)}
        />
      )}
    </section>
  );
}
