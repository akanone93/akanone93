// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Outlet } from "@tanstack/react-router";

/** Public pages (sign-in family): no shell, no organisation (R1). */
export function PublicLayout() {
  return (
    <main className="mx-auto flex min-h-screen max-w-sm flex-col justify-center p-6">
      <Outlet />
    </main>
  );
}

/** Canvas layout: an empty shell until the first canvas page lands (L-7). */
export function CanvasLayout() {
  return (
    <main className="h-screen w-screen">
      <Outlet />
    </main>
  );
}

/** Page layout for app pages: an empty shell until the first app page lands (L-7). */
export function PageLayout() {
  return (
    <div className="flex flex-col gap-6 p-6">
      <Outlet />
    </div>
  );
}
