// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { shellMessages } from "../../shell.messages.ts";
import { notifyMessages } from "./notify.messages.ts";
import { chromeMessages } from "./shell-chrome.messages.ts";

const link = "underline-offset-4 transition-colors hover:text-foreground hover:underline";

/**
 * The foot of every signed-in page: the Studio version loaded in this tab, and the shortcut sheet and documentation
 * the help menu also offers. It closes the page, so the last row of a list never sits on the window's edge.
 */
export function SiteFooter({
  version,
  docsUrl,
}: {
  readonly version: string;
  readonly docsUrl: string | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const help = notifyMessages(i18n);
  return (
    <footer className="site-footer mt-auto flex md:rounded-b-xl flex-wrap items-center gap-x-4 gap-y-1 border-t px-4 py-4 text-xs text-muted-foreground lg:px-6">
      <span className="font-medium text-foreground">{shellMessages(i18n).appTitle}</span>
      {version !== "" && <span className="tabular-nums">{chromeMessages(i18n).version(version)}</span>}
      <nav aria-label={chromeMessages(i18n).footer} className="ms-auto flex items-center gap-4">
        <Link to="/me/shortcuts" className={link}>
          {help.helpShortcuts}
        </Link>
        {docsUrl !== undefined && (
          <a href={docsUrl} target="_blank" rel="noopener noreferrer" className={link}>
            {help.helpDocs}
          </a>
        )}
      </nav>
    </footer>
  );
}
