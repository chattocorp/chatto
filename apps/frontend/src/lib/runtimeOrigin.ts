/** Whether the frontend origin can also host a Chatto HTTP backend. */
export function isBackendCapableOrigin(url: Pick<URL, 'protocol'>): boolean {
  return url.protocol === 'http:' || url.protocol === 'https:';
}

/**
 * Whether a URL hostname names the local device. This matches the server's
 * loopback OAuth callback rule: literal loopback IPs, `localhost`, and valid
 * `.localhost` subdomains.
 */
export function isLoopbackHostname(hostname: string): boolean {
  const host = hostname.toLowerCase();
  if (host === 'localhost' || host === '127.0.0.1' || host === '::1' || host === '[::1]') {
    return true;
  }
  if (!host.endsWith('.localhost')) return false;
  return host
    .slice(0, -'.localhost'.length)
    .split('.')
    .every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label));
}
