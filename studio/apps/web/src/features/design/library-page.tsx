// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { ComponentService } from "@plux/api";
import { Button } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { AdoptDialog } from "../library/adopt-dialog.tsx";
import { designMessages } from "./design.messages.ts";
import { Frame, useDesign } from "./scope.tsx";

/** One component of the organisation's library as this app uses it; adopting a version is D32. */
function Table(): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  const { session } = useApp();
  const client = createClient(ComponentService, session.transport);
  const [target, setTarget] = useState<string | undefined>(undefined);
  const data = useQuery({
    queryKey: ["design-library", ctx.app],
    queryFn: async () => {
      const [items, adoptions] = await Promise.all([
        client.listLibraryComponents({ visibility: "", query: "", page: { pageSize: 200 } }),
        client.listComponentAdoptions({ appId: ctx.app }),
      ]);
      return { items: items.items, adoptions: adoptions.adoptions };
    },
  });
  const versions = useQuery({
    queryKey: ["design-library", "versions", target],
    enabled: target !== undefined,
    queryFn: () => client.listComponentVersions({ id: target ?? "" }),
  });
  if (data.isPending) {
    return (
      <p role="status" className="text-body text-secondary">
        {m.libraryLoading}
      </p>
    );
  }
  if (data.isError) {
    return (
      <p role="alert" className="text-body">
        {m.libraryFailed}
      </p>
    );
  }
  const picked = data.data.items.find((i) => i.id === target);
  return (
    <>
      {data.data.items.length === 0 ? (
        <p className="text-body text-secondary">{m.libraryNone}</p>
      ) : (
        <table aria-label={m.libraryTable} className="w-full text-body">
          <thead>
            <tr className="text-start text-caption text-secondary">
              <th scope="col">{m.colComponent}</th>
              <th scope="col">{m.colAdopted}</th>
              <td />
            </tr>
          </thead>
          <tbody>
            {data.data.items.map((item) => {
              const adopted = data.data.adoptions.find((a) => a.itemId === item.id);
              return (
                <tr key={item.id}>
                  <th scope="row" className="text-start font-normal">
                    {item.name}
                  </th>
                  <td>{adopted === undefined ? m.notAdopted : m.adoptedVersion(adopted.version)}</td>
                  <td>
                    <Button variant="secondary" disabled={!ctx.canEdit} onClick={() => setTarget(item.id)}>
                      {m.adopt(item.name)}
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      {picked !== undefined && versions.data !== undefined && (
        <AdoptDialog
          appId={ctx.app}
          appName={ctx.app}
          versions={versions.data.versions}
          adopted={data.data.adoptions.find((a) => a.itemId === picked.id)}
          adopt={async (appId, version) => {
            const res = await client.adoptComponentVersion({ appId, id: picked.id, version });
            await data.refetch();
            return res.conflicts;
          }}
          onClose={() => setTarget(undefined)}
        />
      )}
    </>
  );
}

/** A19: the organisation's locked component library as this app uses it (STU-112, STU-085). */
export function LibraryPage(): ReactElement {
  const m = designMessages(useLingui().i18n);
  return (
    <Frame id="design-library" title={m.libraryTitle} caption={m.libraryCaption}>
      <Table />
    </Frame>
  );
}
