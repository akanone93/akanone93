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
    cancel: i18n._({
      id: "assets.cancel",
      message: "Cancel",
      comment: "Cancel button of the delete-asset dialog.",
    }),
  };
}
