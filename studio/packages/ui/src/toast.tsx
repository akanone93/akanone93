// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { Toaster as SonnerToaster, toast } from "sonner";

/** U11: toasts last 5 s; errors stay until dismissed. */
export const toastDurationMs = 5000;

export type ToastKind = "success" | "error" | "info";

export interface ToasterProps {
  /** The Studio direction; toasts sit at the bottom end. */
  readonly direction: "ltr" | "rtl";
  /** Accessible name of the toast region, from the message catalogue. */
  readonly regionLabel: string;
}

/** Sonner viewport with token styles instead of its own theme (U11 "Toast"). Mount once near the root. */
export function Toaster({ direction, regionLabel }: ToasterProps): ReactElement {
  return (
    <SonnerToaster
      dir={direction}
      position={direction === "rtl" ? "bottom-left" : "bottom-right"}
      containerAriaLabel={regionLabel}
      duration={toastDurationMs}
      toastOptions={{
        unstyled: true,
        classNames: {
          toast: "flex items-center gap-2 rounded-full bg-glass px-4 py-2 text-body text-primary shadow-e2",
          error: "text-danger",
        },
      }}
    />
  );
}

/** Shows a toast. Errors never time out; the others follow the 5 s default. */
export function notify(kind: ToastKind, message: string): string | number {
  if (kind === "error") {
    return toast.error(message, { duration: Number.POSITIVE_INFINITY });
  }
  return kind === "success" ? toast.success(message) : toast.info(message);
}
