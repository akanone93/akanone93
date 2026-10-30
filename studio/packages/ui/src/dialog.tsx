// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ComponentProps, ReactElement, ReactNode } from "react";
import {
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
  AlertDialog as ShadcnAlertDialog,
} from "./components/alert-dialog";
import { Button } from "./components/button";
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Dialog as ShadcnDialog,
} from "./components/dialog";
import { Icon } from "./icon.tsx";

/** `full` is the full-screen overlay of the heavy editors (STU-063): it fills the viewport and has no corners. */
export type DialogSize = "sm" | "md" | "lg" | "full";

const sizeClass: Readonly<Record<DialogSize, string>> = {
  sm: "sm:max-w-sm",
  md: "sm:max-w-lg",
  lg: "sm:max-w-3xl",
  full: "top-0 start-0 h-full w-full max-w-none translate-x-0 translate-y-0 rounded-none sm:max-w-none rtl:translate-x-0",
};

export interface DialogLabels {
  readonly close: string;
}

export type DialogProps = Omit<ComponentProps<typeof ShadcnDialog>, "children"> & {
  readonly title: string;
  readonly description?: string;
  readonly size?: DialogSize;
  readonly labels: DialogLabels;
  /** The element that opens the dialog; omit when `open` is controlled. */
  readonly trigger?: ReactElement;
  readonly children?: ReactNode;
};

/**
 * shadcn's Dialog (ADR-0103): focus is trapped and returned, Escape closes (STU-011). The close button is shadcn's,
 * named from the message catalogue.
 */
export function Dialog({
  title,
  description,
  size = "md",
  labels,
  trigger,
  children,
  ...root
}: DialogProps): ReactElement {
  return (
    <ShadcnDialog {...root}>
      {trigger !== undefined && <DialogTrigger render={trigger} />}
      <DialogContent showCloseButton={false} className={sizeClass[size]}>
        {/* First in reading order, as the old header had it; shadcn's place on screen, the top end corner. */}
        <DialogClose
          render={<Button variant="ghost" size="icon-sm" className="absolute top-2 end-2" aria-label={labels.close} />}
        >
          <Icon name="x" />
        </DialogClose>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description !== undefined && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {children}
      </DialogContent>
    </ShadcnDialog>
  );
}

export interface AlertDialogLabels {
  readonly cancel: string;
  readonly confirm: string;
}

export type AlertDialogProps = Omit<ComponentProps<typeof ShadcnAlertDialog>, "children"> & {
  readonly title: string;
  readonly description: string;
  readonly labels: AlertDialogLabels;
  readonly onConfirm: () => void;
  readonly trigger?: ReactElement;
};

/** shadcn's AlertDialog for destructive actions: a role `alertdialog` that outside clicks do not dismiss. */
export function AlertDialog({
  title,
  description,
  labels,
  onConfirm,
  trigger,
  ...root
}: AlertDialogProps): ReactElement {
  return (
    <ShadcnAlertDialog {...root}>
      {trigger !== undefined && <AlertDialogTrigger render={trigger} />}
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{labels.cancel}</AlertDialogCancel>
          <AlertDialogCancel variant="destructive" onClick={onConfirm}>
            {labels.confirm}
          </AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </ShadcnAlertDialog>
  );
}
