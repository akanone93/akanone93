// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { TransportProvider } from "@connectrpc/connect-query";
import { directionOf } from "@plux/i18n";
import { StyleNonce, Toaster } from "@plux/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { useState } from "react";
import { notifyMessages } from "./features/shell/notify.messages.ts";
import { useStudioI18n } from "./i18n-provider.tsx";
import { createSession, type Session } from "./lib/session.ts";
import { AppContextProvider } from "./lib/use-app.tsx";
import { detectWebGL2 } from "./lib/webgl.ts";
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
  const [router] = useState(() => providedRouter ?? createAppRouter({ session, webgl2: detectWebGL2() }));
  return (
    <StyleNonce>
      <AppContextProvider value={{ session, studio }}>
        <TransportProvider transport={session.transport}>
          <QueryClientProvider client={session.queryClient}>
            <RouterProvider router={router} />
          </QueryClientProvider>
        </TransportProvider>
      </AppContextProvider>
      {/* Where `notify` shows its toasts: a refused save, a failed import, an action that did not run. */}
      <Toaster direction={directionOf(studio.locale())} regionLabel={notifyMessages(studio.i18n).toasts} />
    </StyleNonce>
  );
}
