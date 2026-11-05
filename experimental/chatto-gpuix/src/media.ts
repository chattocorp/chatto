/** Native media transport: bounded raster reads, no cookies, credentials, or redirects. */
import { execFile } from 'node:child_process';

const MAX_IMAGE_BYTES = 12 * 1024 * 1024;
const RASTER_TYPES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp']);

/** Accept web URLs only. Relative URLs resolve against the connected server. */
export function mediaUrl(raw: string | null | undefined, serverUrl: string): URL | null {
  if (!raw) return null;
  try {
    const url = new URL(raw, serverUrl);
    if (url.username || url.password) return null;
    const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
    return url.protocol === 'https:' || (url.protocol === 'http:' && loopback) ? url : null;
  } catch {
    return null;
  }
}

/** Fetch an image for a visible row. Never include the source URL in errors or logs. */
export async function readImage(url: URL, signal: AbortSignal): Promise<string> {
  const response = await fetch(url, {
    signal: AbortSignal.any([signal, AbortSignal.timeout(20_000)]),
    credentials: 'omit',
    redirect: 'error',
    referrerPolicy: 'no-referrer'
  });
  const mime = response.headers.get('content-type')?.split(';')[0]?.trim().toLowerCase();
  if (!response.ok || !mime || !RASTER_TYPES.has(mime) || !response.body) {
    await response.body?.cancel();
    throw new Error('Image unavailable or unsupported');
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > MAX_IMAGE_BYTES) throw new Error('Image exceeds the preview size limit');
      chunks.push(value);
    }
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
  return `data:${mime};base64,${Buffer.concat(chunks).toString('base64')}`;
}

/** Open an explicitly selected web link with the macOS default application. */
export function openMedia(url: URL): Promise<void> {
  return new Promise((resolve, reject) => {
    execFile('/usr/bin/open', [url.href], (error) => {
      if (error) reject(new Error('Could not open the link'));
      else resolve();
    });
  });
}
