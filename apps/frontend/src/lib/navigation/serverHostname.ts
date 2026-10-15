/** Registered hostname lookup shared by local routes and content links. */
import { serverRegistry } from '$lib/client';

/** Find a registered server by hostname, independent of scheme and port. */
export function serverIdForHostname(hostname: string): string | null {
  for (const server of serverRegistry.servers) {
    try {
      if (new URL(server.url).hostname === hostname) return server.id;
    } catch {
      continue;
    }
  }
  return null;
}
