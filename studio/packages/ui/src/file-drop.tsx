// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type DragEvent, type ReactElement, useState } from "react";
import { cx } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.ts";

export interface FileDropProps {
  /** The title of the zone and the accessible name of its file input. */
  readonly label: string;
  /** A line under the title, such as how to drop or which types are taken. */
  readonly hint?: string | undefined;
  /** The file chosen last, shown in place of the title until another is chosen. */
  readonly chosen?: string | undefined;
  readonly accept?: string | undefined;
  readonly multiple?: boolean;
  readonly disabled?: boolean;
  readonly icon?: IconName;
  /** `zone` is a dashed area for forms and panels; `button` a compact button for toolbars. Both take a drop. */
  readonly variant?: "zone" | "button";
  readonly testId?: string | undefined;
  readonly className?: string | undefined;
  readonly onFiles: (files: readonly File[]) => void;
}

/**
 * The one way Studio takes files (U9): drag them onto the zone, or click it (or focus it and press Enter or Space) to
 * open the system picker. It replaces the browser's bare "Choose file" control, which no theme reaches.
 */
export function FileDrop(props: FileDropProps): ReactElement {
  const { label, hint, chosen, accept, multiple = false, disabled = false, variant = "zone", onFiles } = props;
  const [over, setOver] = useState(false);
  const take = (list: FileList | null) => {
    const files = [...(list ?? [])];
    onFiles(multiple ? files : files.slice(0, 1));
  };
  const onDragOver = (e: DragEvent<HTMLLabelElement>) => {
    if (!disabled && e.dataTransfer.types.includes("Files")) {
      e.preventDefault();
      e.dataTransfer.dropEffect = "copy";
      setOver(true);
    }
  };
  const onDrop = (e: DragEvent<HTMLLabelElement>) => {
    setOver(false);
    if (!disabled && e.dataTransfer.files.length > 0) {
      e.preventDefault();
      take(e.dataTransfer.files);
    }
  };
  const zone = variant === "zone";
  return (
    <label
      data-slot="file-drop"
      data-variant={variant}
      data-over={over ? "" : undefined}
      data-disabled={disabled ? "" : undefined}
      onDragOver={onDragOver}
      onDragLeave={() => setOver(false)}
      onDrop={onDrop}
      className={cx(
        "group/drop relative cursor-pointer border border-dashed transition-colors focus-within:ring-3 focus-within:ring-ring/50",
        "data-over:border-primary data-over:bg-primary/10 data-disabled:cursor-not-allowed data-disabled:opacity-50",
        zone
          ? "flex flex-col items-center justify-center gap-1.5 rounded-xl border-border bg-muted/40 px-4 py-5 text-center hover:border-primary/60 hover:bg-primary/5"
          : "inline-flex h-8 items-center gap-1.5 rounded-lg border-border bg-secondary px-2.5 text-sm font-medium text-secondary-foreground hover:border-primary/60",
        props.className,
      )}
    >
      <input
        type="file"
        className="sr-only"
        aria-label={label}
        accept={accept}
        multiple={multiple}
        disabled={disabled}
        data-testid={props.testId}
        onChange={(e) => {
          take(e.currentTarget.files);
          e.currentTarget.value = "";
        }}
      />
      {zone ? (
        <>
          <span
            aria-hidden="true"
            className="flex size-10 items-center justify-center rounded-full bg-primary/12 text-primary transition-transform group-hover/drop:-translate-y-0.5 group-data-over/drop:scale-110"
          >
            <Icon name={props.icon ?? "cloud-upload"} />
          </span>
          <span className="max-w-full truncate text-sm font-medium">{chosen ?? label}</span>
          {hint !== undefined && <span className="text-xs text-muted-foreground">{hint}</span>}
        </>
      ) : (
        <>
          <Icon name={props.icon ?? "upload"} size="pill" />
          <span className="truncate">{chosen ?? label}</span>
        </>
      )}
    </label>
  );
}
