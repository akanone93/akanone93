// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, type ReactNode, use } from "react";
import type { StudioI18n } from "@plux/i18n";
import type { Session } from "./session.ts";

export interface AppContext {
  readonly session: Session;
  readonly studio: StudioI18n;
}

const Context = createContext<AppContext | null>(null);

export function AppContextProvider({ value, children }: { value: AppContext; children: ReactNode }) {
  return <Context value={value}>{children}</Context>;
}

/** The session and the i18n instance the app root owns. */
export function useApp(): AppContext {
  const value = use(Context);
  if (value === null) {
    throw new Error("useApp outside AppContextProvider");
  }
  return value;
}
