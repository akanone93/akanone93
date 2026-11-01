// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppProjectService } from "@plux/api";
import { Button, Icon } from "@plux/ui";
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@plux/ui/components/item";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { onboardingMessages } from "./onboarding.messages.ts";

/**
 * The sample apps the server embeds (Plux Bank, Plux Express, Starter; F17), each importable with one press as an
 * ordinary import into the organisation (STU-015). The copy gets new identifiers and is the person's own.
 */
export function SampleApps({
  organizationId,
  onImported,
}: {
  readonly organizationId: string;
  /** Receives the id of the app the import created. */
  readonly onImported: (appId: string) => void;
}): ReactElement | null {
  const m = onboardingMessages(useLingui().i18n);
  const { session } = useApp();
  const queryClient = useQueryClient();
  const samples = useQuery({
    queryKey: ["sample-apps"],
    queryFn: () => createClient(AppProjectService, session.transport).listSampleApps({}),
  });
  const [working, setWorking] = useState<string | undefined>(undefined);
  const [refusal, setRefusal] = useState<string | undefined>(undefined);
  const list = samples.data?.samples ?? [];
  if (list.length === 0) {
    return null;
  }
  const run = async (sampleId: string) => {
    setWorking(sampleId);
    setRefusal(undefined);
    try {
      const res = await createClient(AppProjectService, session.transport).importSampleApp({
        organizationId,
        sampleId,
        key: "",
        name: "",
      });
      await queryClient.invalidateQueries({ queryKey: ["apps"] });
      await queryClient.invalidateQueries({ queryKey: ["onboarding"] });
      onImported(res.app?.id ?? "");
    } catch (error) {
      const raw = error instanceof ConnectError ? error.rawMessage : "";
      setRefusal(raw === "" ? m.importFailed : raw);
    } finally {
      setWorking(undefined);
    }
  };
  return (
    <section aria-labelledby="onboarding-samples" className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h3 id="onboarding-samples" className="text-sm font-medium">
          {m.samplesTitle}
        </h3>
        <p className="text-sm text-muted-foreground">{m.samplesCaption}</p>
      </div>
      <ul aria-label={m.samplesLabel} className="flex flex-col gap-2">
        {list.map((sample) => (
          <Item key={sample.id} variant="muted" size="sm" render={<li />}>
            <ItemMedia variant="icon">
              <Icon name="package" />
            </ItemMedia>
            <ItemContent>
              <ItemTitle>{sample.name}</ItemTitle>
              <ItemDescription>{sample.description}</ItemDescription>
            </ItemContent>
            <ItemActions>
              <Button
                variant="secondary"
                size="sm"
                loading={working === sample.id}
                loadingLabel={m.importing}
                disabled={working !== undefined}
                onClick={() => void run(sample.id)}
              >
                {m.importSample(sample.name)}
              </Button>
            </ItemActions>
          </Item>
        ))}
      </ul>
      {refusal !== undefined && (
        <p role="alert" className="text-sm text-destructive">
          {refusal}
        </p>
      )}
    </section>
  );
}
