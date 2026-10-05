/**
 * Validate the address of a Chatto server that a host configures.
 *
 * Returns the parsed URL. Throws when the address is not an HTTP or HTTPS URL
 * or contains credentials. The error does not repeat the address, which can
 * contain secrets.
 */
export function parseServerUrl(value: string): URL {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  return url;
}
