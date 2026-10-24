// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { AlertDialog as BaseAlertDialog } from "@base-ui/react/alert-dialog";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import type { ReactElement, ReactNode } from "react";
import { Button } from "./button.tsx";
import { cx } from "./cx.ts";
import { Icon } from "./icon.tsx";

/** U11 widths 400 / 560 / 800 px, approximated by the container scale until dialog width tokens exist. */
export type DialogSize = "sm" | "md" | "lg";

const sizeClass: Readonly<Record<DialogSize, string>> = {
  sm: "max-w-sm",
  md: "max-w-xl",
  lg: "max-w-3xl",
};

const backdropClass = "fixed inset-0 bg-brand-ink opacity-40";
const viewportClass = "fixed inset-0 flex items-center justify-center p-4";
const popupClass = "flex w-full flex-col gap-3 rounded-xl bg-raised p-6 shadow-e4";
const titleClass = "font-serif text-title text-primary";
const descriptionClass = "text-body text-secondary";

export interface DialogLabels {
  readonly close: string;
}

export type DialogProps = Omit<BaseDialog.Root.Props, "children"> & {
  readonly title: string;
  readonly description?: string;
  readonly size?: DialogSize;
  readonly labels: DialogLabels;
  /** The element that opens the dialog; omit when `open` is controlled. */
  readonly trigger?: ReactElement;
  readonly children?: ReactNode;
};

/** Modal dialog: focus is trapped and returned, Escape closes (STU-011). */
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
    <BaseDialog.Root {...root}>
      {trigger !== undefined && <BaseDialog.Trigger render={trigger} />}
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className={backdropClass} />
        <BaseDialog.Viewport className={viewportClass}>
          <BaseDialog.Popup className={cx(popupClass, sizeClass[size])}>
            <div className="flex items-start justify-between gap-3">
              <BaseDialog.Title className={titleClass}>{title}</BaseDialog.Title>
              <BaseDialog.Close render={<Button variant="ghost" size="sm" aria-label={labels.close} />}>
                <Icon name="x" />
              </BaseDialog.Close>
            </div>
            {description !== undefined && (
              <BaseDialog.Description className={descriptionClass}>{description}</BaseDialog.Description>
            )}
            {children}
          </BaseDialog.Popup>
        </BaseDialog.Viewport>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}

export interface AlertDialogLabels {
  readonly cancel: string;
  readonly confirm: string;
}

export type AlertDialogProps = Omit<BaseAlertDialog.Root.Props, "children"> & {
  readonly title: string;
  readonly description: string;
  readonly labels: AlertDialogLabels;
  readonly onConfirm: () => void;
  readonly trigger?: ReactElement;
};

/** Confirm dialog for destructive actions: a role `alertdialog` that outside clicks do not dismiss (U11). */
export function AlertDialog({
  title,
  description,
  labels,
  onConfirm,
  trigger,
  ...root
}: AlertDialogProps): ReactElement {
  return (
    <BaseAlertDialog.Root {...root}>
      {trigger !== undefined && <BaseAlertDialog.Trigger render={trigger} />}
      <BaseAlertDialog.Portal>
        <BaseAlertDialog.Backdrop className={backdropClass} />
        <BaseAlertDialog.Viewport className={viewportClass}>
          <BaseAlertDialog.Popup className={cx(popupClass, sizeClass.sm)}>
            <BaseAlertDialog.Title className={titleClass}>{title}</BaseAlertDialog.Title>
            <BaseAlertDialog.Description className={descriptionClass}>{description}</BaseAlertDialog.Description>
            <div className="flex justify-end gap-2">
              <BaseAlertDialog.Close render={<Button variant="secondary" />}>{labels.cancel}</BaseAlertDialog.Close>
              <BaseAlertDialog.Close render={<Button variant="danger" onClick={onConfirm} />}>
                {labels.confirm}
              </BaseAlertDialog.Close>
            </div>
          </BaseAlertDialog.Popup>
        </BaseAlertDialog.Viewport>
      </BaseAlertDialog.Portal>
    </BaseAlertDialog.Root>
  );
}
