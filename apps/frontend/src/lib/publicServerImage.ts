import type { Attachment } from 'svelte/attachments';

const supportedPublicServerImageTypes = new Set([
  'image/apng',
  'image/avif',
  'image/gif',
  'image/jpeg',
  'image/png',
  'image/webp'
]);

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
        if (!response.ok || !mediaType || !supportedPublicServerImageTypes.has(mediaType)) return;

        const imageData = await response.blob();
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
