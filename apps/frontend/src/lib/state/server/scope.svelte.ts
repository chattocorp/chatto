import { createContext } from 'svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { ServerStateStore } from './store.svelte';

/**
 * The URL-selected server resources owned by a `/chat/[serverId]` route subtree.
 *
 * A scope never changes while its subtree is mounted. The server layout keys
 * the subtree by store identity, and the registry replaces the store and the
 * connection together when the server, account, or session changes. The
 * subtree then remounts with a new scope. So `serverId`, `connection`, and
 * `connection.queryScope` are constants for every descendant: do not wrap them
 * in `$derived`, and do not fence async work against a change of server or
 * session.
 *
 * Two boundaries still need care:
 * - Privacy resets inside one session. The transport rejects responses that
 *   cross a private-data reset with `StaleResponseError`. Use
 *   `createSessionGuard` when a late result must not change the screen or its
 *   caches.
 * - Route parameters below the server, such as a room, user, or role ID. A
 *   page can stay mounted while they change, so tag async state with the ID.
 */
export interface ServerScope {
  readonly serverId: string;
  readonly connection: ServerConnection;
  readonly store: ServerStateStore;
  /** Whether the keyed route subtree that owns this scope is still mounted. */
  readonly isCurrent: () => boolean;
}

/** Access and provide the current route's server scope. */
export const [useServerScope, provideServerScope] = createContext<ServerScope>();
