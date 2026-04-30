/**
 * Centralized URL routes for E2E tests.
 *
 * All test navigation should use these helpers instead of hardcoding URL strings.
 * This mirrors the production `src/lib/navigation.ts` module but is simplified
 * for test usage (always uses "-" as the home instance segment).
 *
 * When route structure changes, update this file and all tests automatically work.
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

export const space = (spaceId: string) => `/chat/${HOME}/${spaceId}`;
export const room = (spaceId: string, roomId: string) =>
	`/chat/${HOME}/${spaceId}/${roomId}`;
export const thread = (spaceId: string, roomId: string, threadId: string) =>
	`/chat/${HOME}/${spaceId}/${roomId}/${threadId}`;
export const messageLink = (spaceId: string, roomId: string, messageId: string) =>
	`/chat/${HOME}/${spaceId}/${roomId}/m/${messageId}`;

// --- Browse & explore (instance-agnostic) ---

export const spaces = '/chat/spaces';
export const newSpace = '/chat/spaces/new';

// --- Instances ---

export const instances = '/instances';
export const browseRooms = (spaceId: string) => `/chat/${HOME}/${spaceId}/rooms`;
export const threads = (spaceId: string) => `/chat/${HOME}/${spaceId}/threads`;
export const preferences = (spaceId: string) => `/chat/${HOME}/${spaceId}/preferences`;

// --- DMs (unified inbox, not under [instanceId]) ---

export const dm = `/chat/dm`;
export const dmConversation = (conversationId: string) => `/chat/dm/${HOME}/${conversationId}`;

// --- Instance admin ---

export const admin = `/chat/${HOME}/admin`;
export const adminUsers = `/chat/${HOME}/admin/users`;
export const adminUser = (userId: string) => `/chat/${HOME}/admin/users/${userId}`;
export const adminSpaces = `/chat/${HOME}/admin/spaces`;
export const adminSystem = `/chat/${HOME}/admin/system`;
export const adminRoles = `/chat/${HOME}/admin/roles`;
export const adminRole = (roleName: string) => `/chat/${HOME}/admin/roles/${roleName}`;
export const adminInstanceSettings = `/chat/${HOME}/admin/settings/instance`;

// --- Space admin ---

export const spaceAdmin = (spaceId: string, sub?: string) =>
	sub ? `/chat/${HOME}/${spaceId}/admin/${sub}` : `/chat/${HOME}/${spaceId}/admin`;
export const spaceAdminGeneral = (spaceId: string) => spaceAdmin(spaceId, 'general');
export const spaceAdminInvites = (spaceId: string) => spaceAdmin(spaceId, 'invites');
export const spaceAdminRooms = (spaceId: string) => spaceAdmin(spaceId, 'rooms');
export const spaceAdminRoles = (spaceId: string) => spaceAdmin(spaceId, 'roles');
export const spaceAdminRolesNew = (spaceId: string) => spaceAdmin(spaceId, 'roles/new');
export const spaceAdminRole = (spaceId: string, roleName: string) =>
	spaceAdmin(spaceId, `roles/${roleName}`);
export const spaceAdminMembers = (spaceId: string) => spaceAdmin(spaceId, 'members');
export const spaceAdminMember = (spaceId: string, userId: string) =>
	spaceAdmin(spaceId, `members/${userId}`);

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
	chatRedirect: /\/chat\/(-|spaces|notifications|dm)/,
	/** Any space page (excludes /spaces, /admin, /dm, /settings, /notifications) */
	anySpace: /\/chat\/-\/(?!spaces|admin|dm|settings|notifications)[a-zA-Z0-9]+/,
	/** Any room page: /chat/-/{spaceId}/{roomId} */
	anyRoom: /\/chat\/-\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+$/,
	/** Any thread page: /chat/-/{spaceId}/{roomId}/{threadId} */
	anyThread: /\/chat\/-\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+$/,
	/** Any DM conversation: /chat/dm/{instanceSegment}/{id} */
	anyDmConversation: /\/chat\/dm\/[^/]+\/[a-f0-9]+$/,
	/** Any admin user page: /chat/-/admin/users/{id} */
	anyAdminUser: /\/chat\/-\/admin\/users\/[a-zA-Z0-9]+/,
	/** Any non-admin chat route (home instance or instance-agnostic) */
	nonAdmin: /\/chat\/(?:-\/(?!admin)|spaces|notifications|dm)/,
	/** Any space + optional room (used after joinSpace redirect) */
	spaceOrRoom: /\/chat\/-\/[a-zA-Z0-9]+(?:\/[a-zA-Z0-9]+)?$/,
	/** Any space + optional room, allowing query params */
	spaceOrRoomWithQuery: /\/chat\/-\/[a-zA-Z0-9]+(?:\/[a-zA-Z0-9]+)?(?:\?.*)?$/,
	/** Any room with query params (e.g. ?highlight=) */
	anyRoomWithQuery: /\/chat\/-\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+/,
	/** Any DM conversation (alphanumeric IDs) */
	anyDmConversationAlpha: /\/chat\/dm\/[^/]+\/[a-zA-Z0-9]+$/,
	/** Browse rooms page: /chat/-/{spaceId}/rooms */
	browseRooms: /\/chat\/-\/[a-zA-Z0-9]+\/rooms$/,
	/** Email verified redirect */
	emailVerified: /\?email_verified=true/,
};

// --- Remote instance helper ---

/**
 * Build a route for a remote instance (used in multi-instance tests).
 * Unlike the home instance routes above, these use a hostname segment instead of "-".
 */
export const remote = {
	room: (hostname: string, spaceId: string, roomId: string) =>
		`/chat/${hostname}/${spaceId}/${roomId}`,
};
