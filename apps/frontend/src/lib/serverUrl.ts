/** Helpers for the server URLs of the frontend's server catalogue. */

/**
 * Convert a server URL to its canonical HTTP(S) origin, or `null` when it is
 * not an HTTP(S) URL without credentials. Two URLs address the same server
 * when their canonical origins are equal: the comparison ignores case,
 * default ports, paths, and a trailing slash.
 */
export function canonicalServerOrigin(value: string): string | null {
  try {
    const url = new URL(value);
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || url.username || url.password) {
      return null;
    }
    return url.origin;
  } catch {
    return null;
  }
}

/**
 * The host, with a port when it is not the default, of a server URL for
 * display. Returns the value unchanged when it is not a URL.
 */
export function serverHost(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}
