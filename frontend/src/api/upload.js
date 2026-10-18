// Browser side of a video upload: the CRC32C the API declares to storage, and
// the resumable upload that sends the file straight to the bucket. Video bytes
// never pass through the API.

const POLYNOMIAL = 0x82f63b78; // CRC-32C (Castagnoli), reflected, as Cloud Storage uses
const TABLE = (() => {
  const table = new Uint32Array(256);
  for (let i = 0; i < 256; i++) {
    let value = i;
    for (let bit = 0; bit < 8; bit++) value = value & 1 ? (value >>> 1) ^ POLYNOMIAL : value >>> 1;
    table[i] = value >>> 0;
  }
  return table;
})();

const CHUNK = 8 * 1024 * 1024; // a multiple of 256 KiB, as resumable uploads require

/** Streams the file and returns its CRC32C as base64 of four big-endian bytes. */
export async function crc32c(file) {
  let crc = 0xffffffff;
  const reader = file.stream().getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    for (let i = 0; i < value.length; i++) crc = TABLE[(crc ^ value[i]) & 0xff] ^ (crc >>> 8);
  }
  crc = (crc ^ 0xffffffff) >>> 0;
  const bytes = new Uint8Array([(crc >>> 24) & 0xff, (crc >>> 16) & 0xff, (crc >>> 8) & 0xff, crc & 0xff]);
  return btoa(String.fromCharCode(...bytes));
}

export class UploadError extends Error {
  constructor(status) {
    super(`upload failed with HTTP ${status}`);
    this.status = status;
  }
}

/** The next offset after a 308 Range header such as "bytes=0-1048575". */
export function persisted(range) {
  const match = /^bytes=0-(\d+)$/.exec(range || "");
  return match ? Number(match[1]) + 1 : 0;
}

/**
 * Starts the signed resumable upload and sends the file in chunks.
 * `signed` is the API's { method, url, headers }, including the signed
 * x-goog-content-length-range that caps the upload at the declared size;
 * onProgress receives 0 to 1.
 */
export async function upload(signed, file, { onProgress } = {}) {
  const start = await fetch(signed.url, { method: signed.method || "POST", headers: signed.headers || {} });
  if (!start.ok) throw new UploadError(start.status);
  const session = start.headers.get("Location");
  if (!session) throw new UploadError(start.status);

  let offset = 0;
  while (offset < file.size) {
    const end = Math.min(offset + CHUNK, file.size);
    const response = await fetch(session, {
      method: "PUT",
      headers: { "Content-Range": `bytes ${offset}-${end - 1}/${file.size}` },
      body: file.slice(offset, end),
    });
    if (response.status === 308) {
      // Storage reports the bytes it kept as "bytes=0-N"; continue after them.
      // Without a Range header it kept nothing, so the upload restarts at 0.
      offset = persisted(response.headers.get("Range"));
    } else if (response.ok) {
      offset = file.size;
    } else {
      throw new UploadError(response.status);
    }
    onProgress?.(Math.min(offset / file.size, 1));
  }
}

/**
 * The whole browser side of a video upload: checksum, declare, upload and
 * verify. `declare(body)` asks the API for a signed upload and resolves to its
 * response, which carries `upload`; `verify(declared)` finalizes it. onStage
 * receives "checksum", "upload" and "verify"; onProgress 0 to 1 while uploading.
 * Resolves to what verify returned.
 */
export async function sendVideo(file, { declare, verify, onStage, onProgress }) {
  onStage?.("checksum");
  const body = { fileName: file.name, contentType: file.type || "video/mp4", sizeBytes: file.size, crc32c: await crc32c(file) };
  const declared = await declare(body);
  onStage?.("upload");
  await upload(declared.upload, file, { onProgress });
  onStage?.("verify");
  return verify(declared);
}
