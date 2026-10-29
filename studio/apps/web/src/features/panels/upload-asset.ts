// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Client } from "@connectrpc/connect";
import type { AssetService } from "@plux/api";

/** The slice of AssetService the upload needs, so a test can stub it. */
export type UploadClient = Pick<
  Client<typeof AssetService>,
  "beginAssetUpload" | "uploadAssetChunk" | "getAssetUpload" | "finishAssetUpload"
>;

/** One file upload that can be resumed after a failed chunk. */
export interface UploadJob {
  readonly appId: string;
  readonly name: string;
  readonly session: string;
  readonly file: Blob;
}

/** Where an upload stands; kept by the caller so a retry continues instead of restarting. */
export interface UploadState {
  uploadId: string;
  chunkSize: number;
  digest: Uint8Array<ArrayBuffer>;
}

async function sha256(blob: Blob): Promise<Uint8Array<ArrayBuffer>> {
  return new Uint8Array(await crypto.subtle.digest("SHA-256", await blob.arrayBuffer()));
}

/**
 * Uploads a file through BeginAssetUpload, UploadAssetChunk and FinishAssetUpload (SRV-060). Pass the
 * `state` of an earlier failed attempt to resume it: the server's progress is read and sending
 * continues at that offset. `onProgress` gets the bytes the server holds.
 */
export async function uploadAsset(
  client: UploadClient,
  job: UploadJob,
  state: { current: UploadState | null },
  onProgress: (received: number) => void,
) {
  let received = 0;
  if (state.current === null) {
    const digest = await sha256(job.file);
    const begun = await client.beginAssetUpload({
      appId: job.appId,
      name: job.name,
      session: job.session,
      size: BigInt(job.file.size),
      sha256: digest,
    });
    state.current = { uploadId: begun.uploadId, chunkSize: Number(begun.chunkSize), digest };
  } else {
    received = Number((await client.getAssetUpload({ uploadId: state.current.uploadId })).received);
  }
  onProgress(received);
  const { uploadId, chunkSize } = state.current;
  while (received < job.file.size) {
    const chunk = new Uint8Array(await job.file.slice(received, received + chunkSize).arrayBuffer());
    const res = await client.uploadAssetChunk({ uploadId, offset: BigInt(received), chunk });
    received = Number(res.received);
    onProgress(received);
  }
  const done = await client.finishAssetUpload({ uploadId });
  state.current = null;
  return done.asset;
}

/** What an upload failure means to the person, from the Plux code in the server's message (SRV-060). */
export type UploadFailure = "notFound" | "outOfOrder" | "chunkTooLarge" | "tooLarge" | "hashMismatch" | "sizeMismatch";

const failureByCode: Readonly<Record<number, UploadFailure>> = {
  6032: "notFound",
  6033: "outOfOrder",
  6034: "chunkTooLarge",
  6035: "tooLarge",
  6036: "hashMismatch",
  6037: "sizeMismatch",
};

/** The dedicated upload failure an error carries (`PLX-6032` to `PLX-6037`), or null for any other. */
export function uploadFailure(err: unknown): UploadFailure | null {
  const text = err instanceof Error ? err.message : "";
  const m = /PLX-(\d{4})/.exec(text);
  return (m?.[1] !== undefined ? failureByCode[Number(m[1])] : undefined) ?? null;
}
