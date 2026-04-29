/**
 * Bundles all instance-scoped stores into a single class per instance.
 * Created and managed by the InstanceRegistry — do not instantiate directly.
 */

import { CurrentUserState } from '$lib/auth/currentUser.svelte';
import { InstanceState } from './state.svelte';
import type { InstancePermissions, ViewerData } from './permissions.svelte';
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

const DM_SPACE_ID = 'DM';

const EMPTY_PERMISSIONS: InstancePermissions = {
	loaded: false,
	canViewAdmin: false,
	canCreateSpace: false,
	canListSpaces: false,
	canViewDMs: false,
	canWriteDMs: false,
	canAdminViewUsers: false,
	canAdminManageUsers: false,
	canAdminViewSpaces: false,
	canAdminViewRoles: false,
	canAdminManageRoles: false,
	canAdminViewSystem: false,
	canAdminViewAudit: false
};

export class InstanceStateStore {
	readonly instanceId: string;
	readonly currentUser: CurrentUserState;
	readonly instance: InstanceState;
	readonly notifications: NotificationStore;
	readonly roomUnread: RoomUnreadStore;
	readonly notificationLevels: NotificationLevelStore;
	readonly pendingHighlights: PendingHighlightStore;
	readonly voiceCall: VoiceCallState;
	readonly callParticipants: CallParticipantsState;
	readonly activeCallRooms: ActiveCallRoomsState;

	/** Per-instance viewer permissions (loaded by InstanceSpaceSection). */
	permissions = $state<InstancePermissions>(EMPTY_PERMISSIONS);

	/**
	 * Live reference to the registered instance. Reads pick up `updateInstance`
	 * mutations (e.g. token refresh, name change) because the registry stores
	 * instances in $state.
	 */
	readonly #registered: RegisteredInstance;

	constructor(registered: RegisteredInstance, gqlClient: GraphQLClient) {
		this.instanceId = registered.id;
		this.#registered = registered;
		const cookieAuth = this.#cookieAuth;

		const client = gqlClient.client;
		this.currentUser = new CurrentUserState(client, cookieAuth);
		this.instance = new InstanceState(client);
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
	 */
	spaceIndicator(spaceId: string): SpaceIndicator {
		if (this.notifications.hasSpaceNotification(spaceId)) return 'notification';
		if (this.roomUnread.spaceHasUnread(spaceId)) return 'unread';
		return null;
	}

	/**
	 * Indicator for the DM area. DM notifications have no `spaceId` so they
	 * need their own check; unread tracking uses the synthetic `'DM'` space id.
	 */
	dmIndicator(): SpaceIndicator {
		if (this.notifications.hasDMNotifications()) return 'notification';
		if (this.roomUnread.spaceHasUnread(DM_SPACE_ID)) return 'unread';
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
