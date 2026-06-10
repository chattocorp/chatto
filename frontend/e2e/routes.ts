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
 * sit directly under `[serverId]`. Helpers that used to take `spaceId` no
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

/**
 * Back-compat alias: post-#330 PR(a) the Browse Spaces page is gone, so tests
 * that used to navigate there now land on the chat root. Kept as a name so
 * existing call sites compile.
 */
export const spaces = `/chat/${HOME}`;

// Browse Rooms was retired; its functionality is folded into the server
// Overview page at `/chat/{server}/overview`. The export name is kept as
// an alias so existing tests don't need to be renamed in a single sweep.
export const browseRooms = `/chat/${HOME}/overview`;
export const threads = `/chat/${HOME}/threads`;
export const preferences = `/chat/${HOME}/preferences`;

// --- DMs ---
//
// Per #330 phase 3, DMs are now rooms on the Server: they share the same
// URL shape as channel rooms (use the `room(roomId)` helper above) and
// appear in the primary-space sidebar. The dedicated /chat/dm inbox is
// gone for the time being while we re-think the cross-server consolidated
// view.

// --- Server admin (the unified admin surface) ---

export const serverAdmin = (sub?: string) =>
	sub ? `/chat/${HOME}/server-admin/${sub}` : `/chat/${HOME}/server-admin`;
export const serverAdminGeneral = serverAdmin('general');
export const serverAdminRooms = serverAdmin('rooms');
export const serverAdminPermissions = serverAdmin('permissions');
export const serverAdminPermissionsNew = serverAdmin('permissions/new');
export const serverAdminPermission = (roleName: string) =>
  serverAdmin(`permissions/${roleName}`);
// Back-compat aliases for tests and page objects that still use the older
// "roles" naming for the role-permission editor.
export const serverAdminRoles = serverAdminPermissions;
export const serverAdminRolesNew = serverAdminPermissionsNew;
export const serverAdminRole = serverAdminPermission;
export const serverAdminMembers = serverAdmin('members');
export const serverAdminMember = (userId: string) => serverAdmin(`members/${userId}`);
export const serverAdminSecurity = serverAdmin('security');
export const serverAdminSystem = serverAdmin('system');
export const serverAdminMemberPermissions = (userId: string) =>
  serverAdmin(`members/${userId}/permissions`);

// Back-compat aliases — the dedicated /admin route tree was removed once
// instance admin folded into server admin. Existing tests that reference
// the old names keep working via these forward pointers.
export const admin = serverAdminGeneral;
export const adminUsers = serverAdminMembers;
export const adminUser = serverAdminMember;
export const adminSpaces = serverAdminMembers;
export const adminSystem = serverAdminSystem;
export const adminRoles = serverAdminPermissions;
export const adminRole = serverAdminPermission;
// Legacy "instance settings" page motd/welcome/blocked — split across the
// /general (messages) and /security (blocked usernames) tabs now.
export const adminServerSettings = serverAdminGeneral;

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
	chatRedirect: /\/chat\/(-|notifications)/,
	/** Any room page: /chat/-/{roomId} (channels and DMs share this shape post-#330 phase 3). */
	anyRoom: /\/chat\/-\/[a-zA-Z0-9]+$/,
	/** Any thread page: /chat/-/{roomId}/{threadId} */
	anyThread: /\/chat\/-\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+$/,
	/** Any admin user page: /chat/-/server-admin/members/{id} */
	anyAdminUser: /\/chat\/-\/server-admin\/members\/[a-zA-Z0-9]+/,
	/** Any non-admin chat route (home instance or instance-agnostic) */
	nonAdmin: /\/chat\/(?:-(?:\/(?!server-admin)|$)|notifications)/,
	/** Chat root or any room (used after redirects) */
	chatRootOrRoom: /\/chat\/-(?:\/[a-zA-Z0-9]+)?$/,
	/** Chat root or any room, allowing query params */
	chatRootOrRoomWithQuery: /\/chat\/-(?:\/[a-zA-Z0-9]+)?(?:\?.*)?$/,
	/** Any room with query params (e.g. ?highlight=) */
	anyRoomWithQuery: /\/chat\/-\/[a-zA-Z0-9]+/,
	/** Browse rooms — folded into the server overview at /chat/-/overview */
	browseRooms: /\/chat\/-\/overview$/,
	/** Email verified redirect */
	emailVerified: /\?email_verified=true/,

	/**
	 * Back-compat aliases. After ADR-027 there's no separate "space" URL —
	 * the chat URL goes straight from instance to room — so these alias the
	 * post-collapse equivalents to keep older tests working without churn.
	 *
	 * `anySpace` matches either the chat root or any room because, after the
	 * auto-join-on-space-entry behaviour was retired, navigating to the
	 * space root lands on `/chat/-` until the user explicitly enters a
	 * room. Older tests that wait on `anySpace` just care that we've
	 * landed *somewhere* on the server, so the relaxed pattern is the
	 * correct meaning today.
	 */
	get anySpace() {
		return this.chatRootOrRoom;
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
