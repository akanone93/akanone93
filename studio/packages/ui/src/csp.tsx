// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { CSPProvider } from "@base-ui/react/csp-provider";
import type { ReactElement, ReactNode } from "react";

/**
 * The nonce of the page's scripts, which the BFF stamps on every document (studio/apps/bff/src/security.ts). The
 * strict CSP takes `<style>` elements only with it; CodeMirror, Monaco and Base UI reuse it for theirs.
 */
export function pageNonce(): string | undefined {
  if (typeof document === "undefined") {
    return undefined;
  }
  return document.querySelector<HTMLScriptElement>("script[nonce]")?.nonce || undefined;
}

/**
 * Gives Base UI the page's nonce: its select popups and scroll areas render a `<style>` element (hidden scrollbars),
 * which the strict CSP blocks without it. Mount once at the root.
 */
export function StyleNonce({ children }: { readonly children: ReactNode }): ReactElement {
  return <CSPProvider nonce={pageNonce()}>{children}</CSPProvider>;
}
