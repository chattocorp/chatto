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

/** Read the single DNS hostname in a web handoff. Never accept URL components or IP literals. */
export function handoffHostname(query: URLSearchParams): string | null {
  const entries = [...query];
  if (entries.length !== 1 || entries[0][0] !== 'server') return null;
  const value = entries[0][1];
  if (!/^[A-Za-z0-9.-]+$/.test(value)) return null;
  const hostname = value.toLowerCase();
  const labels = hostname.split('.');
  if (
    hostname.length > 253 ||
    labels.length < 2 ||
    labels.some((label) => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label)) ||
    /^\d+$/.test(labels[labels.length - 1])
  )
    return null;

  // URL rejects invalid IDNA and can interpret some DNS-looking inputs as IP addresses.
  try {
    return new URL(`https://${hostname}`).hostname === hostname ? hostname : null;
  } catch {
    return null;
  }
}
