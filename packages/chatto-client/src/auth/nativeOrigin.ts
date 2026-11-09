/** Origin validation shared by native password and browser authorization flows. */
import { parseServerUrl } from '../util/serverUrl.js';

/** Require HTTPS or loopback HTTP, without a path, credentials, query or fragment. */
export function nativeServerOrigin(value: string): URL {
  const origin = parseServerUrl(value);
  const loopback =
    origin.hostname === 'localhost' ||
    origin.hostname === '[::1]' ||
    /^127\.\d+\.\d+\.\d+$/.test(origin.hostname);
  if (
    (origin.protocol !== 'https:' && !loopback) ||
    origin.pathname !== '/' ||
    origin.search ||
    origin.hash
  )
    throw new Error('Use an HTTPS server origin, or HTTP on loopback');
  return origin;
}
