// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import type { UploadFailure } from "./upload-asset.ts";

/** Messages of the Assets tab (S2.6, SRV-060). */
export function assetsMessages(i18n: I18n) {
  return {
    tab: i18n._({
      id: "assets.tab",
      message: "Assets",
      comment: "Label and accessible name of the tab that lists the app's images, fonts and other files.",
    }),
    upload: i18n._({
      id: "assets.upload",
      message: "Upload file",
      comment: "Button that opens the file picker to upload an asset.",
    }),
    empty: i18n._({
      id: "assets.empty",
      message: "No assets yet.",
      comment: "Shown in the Assets tab when the app has no files.",
    }),
    loading: i18n._({
      id: "assets.loading",
      message: "Loading assets…",
      comment: "Shown in the Assets tab while the list loads.",
    }),
    loadFailed: i18n._({
      id: "assets.loadFailed",
      message: "The assets could not be loaded.",
      comment: "Shown in the Assets tab when the list request failed.",
    }),
    uploading: (name: string) =>
      i18n._({
        id: "assets.uploading",
        message: "Uploading {name}",
        values: { name },
        comment: "Accessible name of the progress meter of a file being uploaded; name is the file's name.",
      }),
    progress: (received: string, total: string) =>
      i18n._({
        id: "assets.progress",
        message: "{received} of {total}",
        values: { received, total },
        comment: "Reading of an upload's progress, such as 2 KB of 10 KB.",
      }),
    failed: (name: string) =>
      i18n._({
        id: "assets.failed",
        message: "Uploading {name} failed.",
        values: { name },
        comment: "Shown when a chunk of an upload failed; the upload can be resumed. name is the file's name.",
      }),
    failure: (kind: UploadFailure) => {
      const reasons = {
        notFound: i18n._({
          id: "assets.failure.not-found",
          message: "The upload expired or no longer exists. Upload the file again.",
          comment: "Shown when the server no longer has the upload session, for example after it sat idle too long.",
        }),
        outOfOrder: i18n._({
          id: "assets.failure.out-of-order",
          message: "A part of the file arrived out of order. Try again to continue from where it stopped.",
          comment: "Shown when an upload chunk was sent at the wrong offset.",
        }),
        chunkTooLarge: i18n._({
          id: "assets.failure.chunk-too-large",
          message: "A part of the file was larger than the server accepts. Upload the file again.",
          comment: "Shown when an upload chunk is above the server's chunk size limit.",
        }),
        tooLarge: i18n._({
          id: "assets.failure.too-large",
          message: "The file is larger than the size limit for assets. Choose a smaller file.",
          comment: "Shown when the file is above the maximum asset size.",
        }),
        hashMismatch: i18n._({
          id: "assets.failure.hash-mismatch",
          message: "The file was damaged on the way to the server. Upload it again.",
          comment: "Shown when the SHA-256 of the received bytes differs from the file's.",
        }),
        sizeMismatch: i18n._({
          id: "assets.failure.size-mismatch",
          message: "The server received a different number of bytes than the file has. Upload it again.",
          comment: "Shown when the received size differs from the declared size of the file.",
        }),
      };
      return reasons[kind];
    },
    resume: i18n._({
      id: "assets.resume",
      message: "Resume",
      comment: "Button that continues a failed upload where it stopped.",
    }),
    dismiss: i18n._({
      id: "assets.dismiss",
      message: "Dismiss",
      comment: "Button that drops a failed upload.",
    }),
    delete: (name: string) =>
      i18n._({
        id: "assets.delete",
        message: "Delete {name}",
        values: { name },
        comment: "Accessible name of the button that deletes an asset; name is the asset's name.",
      }),
    deleteTitle: i18n._({
      id: "assets.deleteTitle",
      message: "Delete this asset?",
      comment: "Title of the confirmation before an asset is deleted.",
    }),
    deleteBody: (name: string) =>
      i18n._({
        id: "assets.deleteBody",
        message: "{name} is removed from the app. Pages that use it show a missing image until you pick another.",
        values: { name },
        comment: "Explains what deleting an asset does; name is the asset's name.",
      }),
    confirm: i18n._({
      id: "assets.confirm",
      message: "Delete",
      comment: "Confirm button of the delete-asset dialog.",
    }),
    dropHint: i18n._({
      id: "assets.dropHint",
      message: "Drag files here, or click to choose them",
      comment: "Second line of the Assets tab's drop zone.",
    }),
    search: i18n._({
      id: "assets.search",
      message: "Search assets",
      comment: "Label of the search field of the Assets tab.",
    }),
    kinds: i18n._({
      id: "assets.kinds",
      message: "Kinds of asset",
      comment: "Accessible name of the row of kind filters in the Assets tab.",
    }),
    kind: (kind: "all" | "image" | "vector" | "font" | "animation" | "other") =>
      i18n._({
        id: "assets.kind",
        message:
          "{kind, select, all {All} image {Images} vector {Vectors} font {Fonts} animation {Animations} other {Other}}",
        values: { kind },
        comment: "A kind filter of the Assets tab.",
      }),
    noMatch: i18n._({
      id: "assets.noMatch",
      message: "No asset matches.",
      comment: "Shown in the Assets tab when the search or the kind leaves nothing.",
    }),
    list: i18n._({
      id: "assets.list",
      message: "The app's assets",
      comment: "Accessible name of the grid of asset cards.",
    }),
    cancel: i18n._({
      id: "assets.cancel",
      message: "Cancel",
      comment: "Cancel button of the delete-asset dialog.",
    }),
    uses: (n: number) =>
      i18n._({
        id: "assets.uses",
        message: "{n, plural, one {# use} other {# uses}}",
        values: { n },
        comment: "How many widgets of the plugin's pages show the asset (PC-36).",
      }),
    unused: i18n._({
      id: "assets.unused",
      message: "Unused",
      comment: "Badge: no widget of the plugin's pages shows the asset.",
    }),
    open: (name: string) =>
      i18n._({
        id: "assets.open",
        message: "Show the details of {name}",
        values: { name },
        comment: "Accessible name of an asset card's button that opens its details.",
      }),
    details: (name: string) =>
      i18n._({
        id: "assets.details",
        message: "Details of {name}",
        values: { name },
        comment: "Accessible name of an asset's details panel.",
      }),
    close: i18n._({ id: "assets.close", message: "Close", comment: "Closes an asset's details." }),
    pixels: (w: number, h: number) =>
      i18n._({
        id: "assets.pixels",
        message: "{w} × {h} px",
        values: { w, h },
        comment: "An image's size in pixels.",
      }),
    hash: i18n._({ id: "assets.hash", message: "SHA-256", comment: "Label of an asset's content hash (AST-001)." }),
    variants: i18n._({
      id: "assets.variants",
      message: "Variants",
      comment: "Label of the processed forms of an image, such as WebP at 1x, 2x and 3x.",
    }),
    processing: (state: string) =>
      i18n._({
        id: "assets.processing",
        message: "{state, select, pending {Processing} failed {Processing failed} other {Ready}}",
        values: { state },
        comment: "Whether the server has made the asset's variants.",
      }),
    selectUses: i18n._({
      id: "assets.selectUses",
      message: "Select its uses",
      comment: "Button that selects every widget showing the asset on the canvas.",
    }),
    replace: i18n._({
      id: "assets.replace",
      message: "Replace…",
      comment: "Button that uploads another file and points the asset's uses at it.",
    }),
    replaceHint: i18n._({
      id: "assets.replaceHint",
      message: "Replacing points its uses in this plugin at the new file; the old file stays until you delete it.",
      comment: "Explains what Replace does.",
    }),
    replaced: (name: string) =>
      i18n._({
        id: "assets.replaced",
        message: "Its uses now show {name}.",
        values: { name },
        comment: "Shown after an asset was replaced; name is the new file.",
      }),
    tooBig: (name: string, size: string, max: string) =>
      i18n._({
        id: "assets.tooBig",
        message: "{name} is {size}, above the {max} one file may be (asset.fileSize). It was not uploaded.",
        values: { name, size, max },
        comment: "A file larger than the per-asset limit is refused before it is uploaded (AST-003).",
      }),
    budget: i18n._({
      id: "assets.budget",
      message: "Assets this plugin uses",
      comment: "Label of the meter of the bytes of the assets the plugin's pages use against the plugin limit.",
    }),
    budgetText: (total: string, max: string) =>
      i18n._({
        id: "assets.budgetText",
        message: "{total} of {max}",
        values: { total, max },
        comment: "The plugin's asset bytes against plugin.assetBytes, e.g. 1.2 MB of 20 MB.",
      }),
    budgetOver: i18n._({
      id: "assets.budgetOver",
      message: "Above plugin.assetBytes: publishing this plugin fails until it uses fewer or smaller assets.",
      comment: "Shown when the plugin's assets weigh more than the limit (AST-003).",
    }),
    budgetNear: i18n._({
      id: "assets.budgetNear",
      message: "Close to plugin.assetBytes: publishing warns about it.",
      comment: "Shown when the plugin's assets pass the warning threshold of the limit.",
    }),
    deleteUsed: (name: string, n: number) =>
      i18n._({
        id: "assets.deleteUsed",
        message:
          "{name} is shown by {n, plural, one {# widget} other {# widgets}} of this plugin. Deleting it leaves them without an image until you pick another.",
        values: { name, n },
        comment: "Delete confirmation of an asset that the plugin's pages use.",
      }),
  };
}
