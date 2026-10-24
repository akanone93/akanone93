// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { useState } from "react";
import { useStudioI18n } from "./i18n-provider.tsx";
import { createSession, type Session } from "./lib/session.ts";
import { AppContextProvider } from "./lib/use-app.tsx";
import { createAppRouter } from "./router.tsx";

/** The Studio application root: owns the session and the router; nothing is module-level. */
export function App({
  session: provided,
  router: providedRouter,
}: {
  session?: Session;
  router?: ReturnType<typeof createAppRouter>;
}) {
  const studio = useStudioI18n();
  const [session] = useState(() => provided ?? createSession("/api"));
  const [router] = useState(() => providedRouter ?? createAppRouter({ session }));
  return (
    <AppContextProvider value={{ session, studio }}>
      <TransportProvider transport={session.transport}>
        <QueryClientProvider client={session.queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </TransportProvider>
    </AppContextProvider>
  );
}
