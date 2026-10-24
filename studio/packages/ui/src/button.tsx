// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button as BaseButton } from "@base-ui/react/button";
import type { ComponentPropsWithoutRef, ReactElement } from "react";
import { cx, focusRing } from "./cx.ts";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md";

const variantClass: Readonly<Record<ButtonVariant, string>> = {
  primary: "bg-accent text-on-accent shadow-primary hover:bg-accent-hover",
  secondary: "bg-transparent text-secondary hover:bg-hover hover:text-primary",
  ghost: "bg-transparent text-secondary hover:bg-hover hover:text-primary",
  danger: "bg-danger-fill text-on-accent",
};

const sizeClass: Readonly<Record<ButtonSize, string>> = {
  sm: "h-(--density-field) px-3",
  md: "h-(--density-capsule-button) px-4",
};

export type ButtonProps = Omit<ComponentPropsWithoutRef<typeof BaseButton>, "className"> & {
  readonly variant?: ButtonVariant;
  readonly size?: ButtonSize;
  /** Busy state: the button stays focusable, ignores presses and announces `aria-busy`. */
  readonly loading?: boolean;
  /** Visually hidden text announced while loading, from the message catalogue. */
  readonly loadingLabel?: string;
  readonly className?: string;
};

/** Pill button (U11). Disabled and loading buttons stay focusable so keyboard users keep their place. */
export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  loadingLabel,
  disabled,
  className,
  children,
  ...rest
}: ButtonProps): ReactElement {
  return (
    <BaseButton
      {...rest}
      disabled={disabled === true || loading}
      focusableWhenDisabled
      aria-busy={loading || undefined}
      data-variant={variant}
      className={cx(
        "inline-flex items-center justify-center gap-2 rounded-full text-label font-medium transition-colors duration-fast",
        "disabled:opacity-50 aria-disabled:opacity-50",
        focusRing,
        variantClass[variant],
        sizeClass[size],
        className,
      )}
    >
      {loading && (
        <span aria-hidden="true" className="size-3 animate-spin rounded-full border-2 border-t-transparent" />
      )}
      {loading && loadingLabel !== undefined && <span className="sr-only">{loadingLabel}</span>}
      {children}
    </BaseButton>
  );
}
