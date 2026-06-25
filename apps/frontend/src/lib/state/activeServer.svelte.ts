import { page } from '$app/state';
import { segmentToServerId } from '$lib/navigation';
import { serverRegistry } from './server/registry.svelte';

/**
 * Returns the active server ID, derived from the URL `[serverId]` segment.
 * Falls back to the origin server when the URL has no segment (or the
 * "-" placeholder) and the origin is registered.
 *
 * Reactive when called inside `$derived` / `$effect` / template — the
 * `page.params` and `serverRegistry.originServer` reads track via Svelte's
 * normal reactivity. No context, no getter dance: just a function that
 * resolves the value on every call.
 */
export function getActiveServer(): string {
  return (
    segmentToServerId(page.params.serverId ?? '-')
    ?? serverRegistry.originServer?.id
    ?? ''
  );
}
