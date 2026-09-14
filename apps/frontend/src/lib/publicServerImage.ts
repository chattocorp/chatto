import type { Attachment } from 'svelte/attachments';

const supportedPublicServerImageTypes = new Set([
  'image/apng',
  'image/avif',
  'image/gif',
  'image/jpeg',
  'image/png',
  'image/webp'
]);

/** Maximum bytes retained for one untrusted public server image. */
export const MAX_PUBLIC_SERVER_IMAGE_BYTES = 5 * 1024 * 1024;

async function readBoundedImage(response: Response, mediaType: string): Promise<Blob | null> {
  const declaredSize = Number(response.headers.get('content-length'));
  if (Number.isFinite(declaredSize) && declaredSize > MAX_PUBLIC_SERVER_IMAGE_BYTES) {
    await response.body?.cancel();
    return null;
  }

  if (!response.body) {
    const imageData = await response.blob();
    return imageData.size <= MAX_PUBLIC_SERVER_IMAGE_BYTES ? imageData : null;
  }

  const reader = response.body.getReader();
  const chunks: ArrayBuffer[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_PUBLIC_SERVER_IMAGE_BYTES) {
        await reader.cancel();
        return null;
      }
      const chunk = new Uint8Array(value.byteLength);
      chunk.set(value);
      chunks.push(chunk.buffer);
    }
  } finally {
    reader.releaseLock();
  }
  return new Blob(chunks, { type: mediaType });
}

/**
 * Resolve a public profile image only when it belongs to the advertised server
 * origin. Returns `null` for unsupported, credentialed, or external URLs.
 */
export function publicServerImageURL(serverOrigin: string, source: string | null): string | null {
  if (!source) return null;

  try {
    const server = new URL(serverOrigin);
    const image = new URL(source, server);
    if (!['http:', 'https:'].includes(server.protocol)) return null;
    if (server.username || server.password || image.username || image.password) return null;
    return image.origin === server.origin ? image.href : null;
  } catch {
    return null;
  }
}

/**
 * Load a public server image without credentials, referrer data, or redirects.
 * The response must identify itself as a supported image type before it reaches
 * the element.
 */
export function loadPublicServerImage(source: string): Attachment<HTMLImageElement> {
  return (image) => {
    const controller = new AbortController();
    let objectURL: string | null = null;
    image.removeAttribute('src');

    void (async () => {
      try {
        const response = await fetch(source, {
          credentials: 'omit',
          redirect: 'error',
          referrerPolicy: 'no-referrer',
          signal: controller.signal
        });
        const mediaType = response.headers
          .get('content-type')
          ?.split(';', 1)[0]
          ?.trim()
          .toLowerCase();
        if (!response.ok || !mediaType || !supportedPublicServerImageTypes.has(mediaType)) {
          await response.body?.cancel();
          return;
        }

        const imageData = await readBoundedImage(response, mediaType);
        if (!imageData) return;
        if (controller.signal.aborted) return;
        objectURL = URL.createObjectURL(imageData);
        image.src = objectURL;
      } catch {
        // Keep the existing image placeholder when the remote image is unavailable.
      }
    })();

    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  };
}
