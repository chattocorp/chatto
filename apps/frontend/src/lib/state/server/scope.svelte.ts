import { createContext } from 'svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { ServerStateStore } from './store.svelte';

/** The URL-selected server resources owned by a `/chat/[serverId]` route subtree. */
export interface ServerScope {
  readonly serverId: string;
  readonly connection: ServerConnection;
  readonly store: ServerStateStore;
}

/** Access and provide the current route's reactive server scope. */
export const [useServerScope, provideServerScope] = createContext<ServerScope>();
