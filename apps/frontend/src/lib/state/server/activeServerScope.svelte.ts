import { serverIdToSegment } from '$lib/navigation';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry, type RegisteredServer } from './registry.svelte';
import { useTrackedConnection } from './connection.svelte';
import type { ServerConnection } from './serverConnection.svelte';
import type { ServerStateStore } from './store.svelte';

/**
 * Reactive facade for the URL-selected server and its per-server state.
 *
 * Route-scoped code should use this instead of open-coding an active-server
 * registry lookup. The facade also makes the contextual connection track the
 * URL server even though the low-level `useConnection()` helper intentionally
 * returns an untracked getter.
 */
export function useActiveServerScope() {
	const id = $derived(getActiveServer());
	let getConnection: (() => ServerConnection) | null = null;
	try {
		getConnection = useTrackedConnection(() => id);
	} catch {
		// Some active-server UI (for example isolated sidebar tests) only needs
		// server id/store/segment and is not rendered under a connection provider.
	}
	const segment = $derived(serverIdToSegment(id));
	const registered = $derived(serverRegistry.getServer(id));
	const store = $derived(serverRegistry.getStore(id));

	return {
		get id(): string {
			return id;
		},
		get segment(): string {
			return segment;
		},
		get registered(): RegisteredServer | undefined {
			return registered;
		},
		get connection(): ServerConnection {
			if (!getConnection) {
				throw new Error('Active server connection context is not available');
			}
			return getConnection();
		},
		get store(): ServerStateStore {
			return store;
		},
		get currentUser() {
			return store.currentUser;
		},
		get serverInfo() {
			return store.serverInfo;
		},
		get notifications() {
			return store.notifications;
		},
		get roomUnread() {
			return store.roomUnread;
		},
		get rooms() {
			return store.rooms;
		},
		get pendingHighlights() {
			return store.pendingHighlights;
		},
		get voiceCall() {
			return store.voiceCall;
		},
		get activeCallRooms() {
			return store.activeCallRooms;
		},
		get callParticipants() {
			return store.callParticipants;
		}
	};
}

export type ActiveServerScope = ReturnType<typeof useActiveServerScope>;
