/**
 * Process-wide directory of server IDs.
 *
 * Each client has its own registry, but some state stays process-wide and is
 * keyed by server ID: the snapshot query cache, user stores, and presence
 * preferences. A server ID therefore belongs to exactly one registry at a
 * time. Registries claim an ID when they add a server and release it when
 * they remove it.
 */

import { ReactiveMap, untrack } from '../reactivity/index.js';
import type { RegisteredServer } from './registry.js';

/** The registry side of the directory; see {@link claimServerId}. */
export interface ServerIdOwner {
  getServer(serverId: string): RegisteredServer | undefined;
}

/** Reactive, so that a lookup made before a server registers updates. */
const owners = new ReactiveMap<string, ServerIdOwner>();

/** Record that `owner` holds `serverId`. Throws when another owner holds it. */
export function claimServerId(serverId: string, owner: ServerIdOwner): void {
  const current = untrack(() => owners.get(serverId));
  if (current && current !== owner) {
    throw new Error(`Server ID "${serverId}" belongs to another Chatto client`);
  }
  owners.set(serverId, owner);
}

/** Release `serverId` if `owner` holds it. */
export function releaseServerId(serverId: string, owner: ServerIdOwner): void {
  if (untrack(() => owners.get(serverId)) === owner) owners.delete(serverId);
}

/** Whether any client holds `serverId`. Not reactive. */
export function isServerIdClaimed(serverId: string): boolean {
  return untrack(() => owners.has(serverId));
}

/** The registry that holds `serverId`, if any. Reactive. */
export function ownerOfServerId(serverId: string): ServerIdOwner | undefined {
  return owners.get(serverId);
}
