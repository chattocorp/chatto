/**
 * Centralized URL routes for E2E tests.
 *
 * All test navigation should use these helpers instead of hardcoding URL strings.
 * This mirrors the production `src/lib/navigation.ts` module but is simplified
 * for test usage (always uses "-" as the home instance segment).
 *
 * When route structure changes, update this file and all tests automatically work.
 *
 * Post-ADR-027: the URL no longer carries a `[spaceId]` segment — chat routes
 * sit directly under `[instanceId]`. Helpers that used to take `spaceId` no
 * longer do; the active space is implicit (the deployment's primary).
 */

/** URL segment for the home (local) instance. */
const HOME = '-';

// --- Root routes ---

export const root = '/';

// --- Auth routes (no instance prefix) ---

export const login = '/login';
export const register = '/register';
export const registerComplete = (token: string) => `/register/complete?token=${token}`;
export const forgotPassword = '/forgot-password';
export const resetPassword = (token: string) => `/reset-password?token=${token}`;
export const loginResetSuccess = '/login?reset=success';
export const joinSpace = (spaceId: string) => `/join/${spaceId}`;

// --- Chat routes (home instance) ---

export const chat = `/chat/${HOME}`;

/**
 * The chat root for the home instance. Equivalent to a "the server" landing
 * page since each instance now has a single primary space.
 */
export const space = () => `/chat/${HOME}`;
export const room = (roomId: string) => `/chat/${HOME}/${roomId}`;
export const thread = (roomId: string, threadId: string) =>
	`/chat/${HOME}/${roomId}/${threadId}`;
export const messageLink = (roomId: string, messageId: string) =>
	`/chat/${HOME}/${roomId}/m/${messageId}`;

// --- Browse & explore (instance-agnostic) ---

export const spaces = '/chat/spaces';

export const browseRooms = `/chat/${HOME}/rooms`;
export const threads = `/chat/${HOME}/threads`;
export const preferences = `/chat/${HOME}/preferences`;

// --- DMs ---
//
// Per #330 phase 3, DMs are now rooms on the Server: they share the same
// URL shape as channel rooms (use the `room(roomId)` helper above) and
// appear in the primary-space sidebar. The dedicated /chat/dm inbox is
// gone for the time being while we re-think the cross-server consolidated
// view.

// --- Instance admin ---

export const admin = `/chat/${HOME}/admin`;
export const adminUsers = `/chat/${HOME}/admin/users`;
export const adminUser = (userId: string) => `/chat/${HOME}/admin/users/${userId}`;
export const adminSpaces = `/chat/${HOME}/admin/spaces`;
export const adminSystem = `/chat/${HOME}/admin/system`;
export const adminRoles = `/chat/${HOME}/admin/roles`;
export const adminRole = (roleName: string) => `/chat/${HOME}/admin/roles/${roleName}`;
export const adminInstanceSettings = `/chat/${HOME}/admin/settings/instance`;

// --- Server admin (was: space admin) ---

export const serverAdmin = (sub?: string) =>
	sub ? `/chat/${HOME}/server-admin/${sub}` : `/chat/${HOME}/server-admin`;
export const serverAdminGeneral = serverAdmin('general');
export const serverAdminInvites = serverAdmin('invites');
export const serverAdminRooms = serverAdmin('rooms');
export const serverAdminRoles = serverAdmin('roles');
export const serverAdminRolesNew = serverAdmin('roles/new');
export const serverAdminRole = (roleName: string) => serverAdmin(`roles/${roleName}`);
export const serverAdminMembers = serverAdmin('members');
export const serverAdminMember = (userId: string) => serverAdmin(`members/${userId}`);

// --- User settings ---

export const settings = `/chat/${HOME}/settings`;
export const settingsAccount = `/chat/${HOME}/settings/account`;
export const settingsNotifications = `/chat/${HOME}/settings/notifications`;
export const settingsPreferences = `/chat/${HOME}/settings/preferences`;

// --- Notifications ---

export const notifications = '/chat/notifications';

// --- URL patterns for waitForURL / assertions ---

export const patterns = {
	/** Any chat route after login redirect (home instance routes or instance-agnostic pages) */
	chatRedirect: /\/chat\/(-|spaces|notifications)/,
	/** Any room page: /chat/-/{roomId} (channels and DMs share this shape post-#330 phase 3). */
	anyRoom: /\/chat\/-\/[a-zA-Z0-9]+$/,
	/** Any thread page: /chat/-/{roomId}/{threadId} */
	anyThread: /\/chat\/-\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+$/,
	/** Any admin user page: /chat/-/admin/users/{id} */
	anyAdminUser: /\/chat\/-\/admin\/users\/[a-zA-Z0-9]+/,
	/** Any non-admin chat route (home instance or instance-agnostic) */
	nonAdmin: /\/chat\/(?:-(?:\/(?!admin)|$)|spaces|notifications)/,
	/** Chat root or any room (used after redirects) */
	chatRootOrRoom: /\/chat\/-(?:\/[a-zA-Z0-9]+)?$/,
	/** Chat root or any room, allowing query params */
	chatRootOrRoomWithQuery: /\/chat\/-(?:\/[a-zA-Z0-9]+)?(?:\?.*)?$/,
	/** Any room with query params (e.g. ?highlight=) */
	anyRoomWithQuery: /\/chat\/-\/[a-zA-Z0-9]+/,
	/** Browse rooms page: /chat/-/rooms */
	browseRooms: /\/chat\/-\/rooms$/,
	/** Email verified redirect */
	emailVerified: /\?email_verified=true/,

	/**
	 * Back-compat aliases. After ADR-027 there's no separate "space" URL —
	 * the chat URL goes straight from instance to room — so these alias the
	 * post-collapse equivalents to keep older tests working without churn.
	 */
	get anySpace() {
		return this.anyRoom;
	},
	get spaceOrRoom() {
		return this.chatRootOrRoom;
	},
	get spaceOrRoomWithQuery() {
		return this.chatRootOrRoomWithQuery;
	},
};

// --- Remote instance helper ---

/**
 * Build a route for a remote instance (used in multi-instance tests).
 * Unlike the home instance routes above, these use a hostname segment instead of "-".
 */
export const remote = {
	room: (hostname: string, roomId: string) => `/chat/${hostname}/${roomId}`,
};
