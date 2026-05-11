/**
 * Bundles all instance-scoped stores into a single class per instance.
 * Created and managed by the ServerRegistry — do not instantiate directly.
 */

import { CurrentUserState } from '$lib/auth/currentUser.svelte';
import { InstanceState } from './state.svelte';
import type { ServerPermissions, ViewerData } from './permissions.svelte';
import { NotificationStore } from './notifications.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { NotificationLevelStore } from './notificationLevel.svelte';
import { PendingHighlightStore } from './pendingHighlight.svelte';
import { VoiceCallState } from './voiceCall.svelte';
import { CallParticipantsState } from './callParticipants.svelte';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';
import type { GraphQLClient } from './graphqlClient.svelte';
import type { RegisteredInstance } from './registry.svelte';

/**
 * What kind of indicator dot a space (or the DM area) should display.
 * - 'notification' = orange dot, has a pending mention/reply/room-message
 * - 'unread' = grey dot, has unread rooms but no pending notification
 * - null = no indicator
 */
export type SpaceIndicator = 'notification' | 'unread' | null;

const EMPTY_PERMISSIONS: ServerPermissions = {
	loaded: false,
	canViewAdmin: false,
	canViewDMs: false,
	canWriteDMs: false,
	canAdminViewUsers: false,
	canAdminManageUsers: false,
	canAdminViewRoles: false,
	canAdminManageRoles: false,
	canAdminViewSystem: false,
	canAdminViewAudit: false
};

export class ServerStateStore {
	readonly serverId: string;
	readonly currentUser: CurrentUserState;
	readonly instance: InstanceState;
	readonly notifications: NotificationStore;
	readonly roomUnread: RoomUnreadStore;
	readonly notificationLevels: NotificationLevelStore;
	readonly pendingHighlights: PendingHighlightStore;
	readonly voiceCall: VoiceCallState;
	readonly callParticipants: CallParticipantsState;
	readonly activeCallRooms: ActiveCallRoomsState;

	/** Per-instance viewer permissions (loaded by ServerSpaceSection). */
	permissions = $state<ServerPermissions>(EMPTY_PERMISSIONS);

	/**
	 * Live reference to the registered instance. Reads pick up `updateServer`
	 * mutations (e.g. token refresh, name change) because the registry stores
	 * instances in $state.
	 */
	readonly #registered: RegisteredInstance;

	constructor(registered: RegisteredInstance, gqlClient: GraphQLClient) {
		this.serverId = registered.id;
		this.#registered = registered;
		const cookieAuth = this.#cookieAuth;

		const client = gqlClient.client;
		this.currentUser = new CurrentUserState(client, cookieAuth);
		this.instance = new InstanceState(client, registered.url);
		this.notifications = new NotificationStore(client);
		this.roomUnread = new RoomUnreadStore();
		this.notificationLevels = new NotificationLevelStore();
		this.pendingHighlights = new PendingHighlightStore();
		this.voiceCall = new VoiceCallState(client);
		this.callParticipants = new CallParticipantsState(client);
		this.activeCallRooms = new ActiveCallRoomsState(client, this.voiceCall);

		// Gate session-revalidation and auth-failure dispatch to cookie-auth
		// instances only. Bearer auth's `handleAuthFailure` would clear
		// `currentUser.user` while leaving the bearer token intact, producing
		// an inconsistent state where `isAuthenticated` (token != null) is
		// still true but the user is gone. Until the data model has a clean
		// way to represent "remote with revoked token", keep the existing
		// behavior of letting the next failed query surface the error.
		if (cookieAuth) {
			gqlClient.setAuthHandlers({
				onAuthFailure: () => this.currentUser.handleAuthFailure(),
				onSessionValidation: () => this.currentUser.validateSession()
			});
		}
	}

	/**
	 * Whether this instance uses cookie auth (origin) vs bearer auth (remote).
	 * Read from the live registered instance so it stays correct if the token
	 * field is ever updated.
	 */
	get #cookieAuth(): boolean {
		return this.#registered.token === null;
	}

	/**
	 * Whether this instance currently has an authenticated user.
	 * - Cookie auth (origin): true when `currentUser.user` is set.
	 * - Bearer auth (remote): true when an access token is registered.
	 */
	get isAuthenticated(): boolean {
		if (this.#cookieAuth) {
			return this.currentUser.user != null;
		}
		return this.#registered.token != null;
	}

	/** Update permissions from viewer query data. */
	setPermissions(viewer: ViewerData): void {
		this.permissions = { ...viewer, loaded: true };
	}

	/**
	 * Single source of truth for the space-level indicator dot.
	 * Notifications take precedence over plain unread.
	 *
	 * For the primary space (issue #330 / ADR-027), DM activity also rolls
	 * up here — DMs are surfaced as rooms on the Server in the merged
	 * sidebar, so the user expects the server icon to light up the same
	 * way it would for a channel mention or unread.
	 */
	spaceIndicator(_spaceId?: string): SpaceIndicator {
		// Post-PR(b) the API has only one server, so spaceId is ignored.
		// Channel + DM activity both roll up to the single server indicator.
		if (this.notifications.hasSpaceNotification()) return 'notification';
		if (this.notifications.hasDMNotifications()) return 'notification';
		if (this.roomUnread.hasAnyUnread) return 'unread';
		return null;
	}

	/**
	 * Indicator for the DM area only. Kept for the ServerSpaceSection's
	 * space-icon click logic that wants the DM-only answer when promoting
	 * DM activity into the primary-space indicator.
	 */
	dmIndicator(): SpaceIndicator {
		if (this.notifications.hasDMNotifications()) return 'notification';
		// We no longer track DM unread separately — `hasAnyUnread` covers it.
		return null;
	}

	/** Clean up resources. */
	dispose(): void {
		this.roomUnread.clear();
		this.notificationLevels.clear();
		this.pendingHighlights.clear();
		this.activeCallRooms.clear();
		this.callParticipants.clear();
	}
}
