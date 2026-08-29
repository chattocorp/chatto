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
 * longer do; the server itself is the chat scope.
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

export const room = (roomId: string) => `/chat/${HOME}/${roomId}`;
export const thread = (roomId: string, threadId: string) => `/chat/${HOME}/${roomId}/${threadId}`;
export const messageLink = (roomId: string, messageId: string) =>
  `/chat/${HOME}/${roomId}/m/${messageId}`;

// --- Server navigation ---

export const serverOverview = `/chat/${HOME}/overview`;
export const threads = `/chat/${HOME}/threads`;
export const preferences = `/chat/${HOME}/preferences`;

// --- DMs ---
//
// Per #330 phase 3, DMs are now rooms on the Server: they share the same
// URL shape as channel rooms (use the `room(roomId)` helper above) and
// appear in the primary-server sidebar. The dedicated /chat/dm inbox is
// gone for the time being while we re-think the cross-server consolidated
// view.

// --- Management surfaces ---

export const serverAdmin = (sub?: string) =>
  sub ? `/chat/${HOME}/manage/server/${sub}` : `/chat/${HOME}/manage/server`;
export const serverAdminGeneral = serverAdmin('general');
export const serverAdminRooms = `/chat/${HOME}/manage/rooms`;
export const serverAdminPermissions = serverAdmin('permissions');
export const serverAdminPermissionsNew = serverAdmin('permissions/new');
export const serverAdminPermission = (roleName: string) => serverAdmin(`permissions/${roleName}`);
export const serverAdminMembers = serverAdmin('members');
export const serverAdminMember = (userId: string) => serverAdmin(`members/${userId}`);
export const serverAdminMemberDelete = (userId: string) =>
  serverAdmin(`members/${userId}/delete`);
export const serverAdminBots = serverAdmin('bots');
export const serverAdminSecurity = serverAdmin('security');
export const serverAdminSystem = serverAdmin('system');
export const serverAdminMemberPermissions = (userId: string) =>
  serverAdmin(`members/${userId}/permissions`);

// --- User settings ---

/** The canonical Settings entry point. */
export const settingsRoot = `/chat/${HOME}/settings`;
export const settingsProfile = `/chat/${HOME}/settings/profile`;
/** The canonical default page for user-settings test flows. */
export const settings = settingsProfile;
export const settingsAccount = `/chat/${HOME}/settings/account`;
export const settingsNotifications = `/chat/${HOME}/settings/notifications`;
export const settingsTime = `/chat/${HOME}/settings/time`;
export const settingsAppearance = `/chat/${HOME}/settings/appearance`;
export const settingsLanguage = `/chat/${HOME}/settings/language`;
export const settingsComposer = `/chat/${HOME}/settings/composer`;
// Legacy App Preferences routes remain available as authenticated redirects
// and as the standalone no-server fallback.
export const appPreferences = '/chat/preferences';
export const appPreferencesLanguage = '/chat/preferences/language';
export const appPreferencesComposer = '/chat/preferences/composer';

// --- Notifications ---

export const notifications = '/chat/notifications';

// --- URL patterns for waitForURL / assertions ---

export const patterns = {
  /** Any chat route after login redirect (home instance routes or instance-agnostic pages) */
  chatRedirect: /\/chat\/(-|notifications|preferences)/,
  /** Any room page: /chat/-/{roomId} (channels and DMs share this shape post-#330 phase 3). */
  anyRoom: /\/chat\/-\/(?!manage$)[a-zA-Z0-9]+$/,
  /** Any thread page: /chat/-/{roomId}/{threadId} */
  anyThread: /\/chat\/-\/(?!manage\/)[a-zA-Z0-9]+\/[a-zA-Z0-9]+$/,
  /** Any admin user page: /chat/-/manage/server/members/{id} */
  anyAdminUser: /\/chat\/-\/manage\/server\/members\/[a-zA-Z0-9]+/,
  /** Any bot-management page: /chat/-/manage/server/bots/{id} */
  anyAdminBot: /\/chat\/-\/manage\/server\/bots\/[a-zA-Z0-9]+$/,
  /** Any non-admin chat route (home instance or instance-agnostic) */
  nonAdmin: /\/chat\/(?:-(?:\/(?!manage(?:\/|$))|$)|notifications|preferences)/,
  /** Chat root or any room (used after redirects) */
  chatRootOrRoom: /\/chat\/-(?:\/(?!manage$)[a-zA-Z0-9]+)?$/,
  /** Chat root or any room, allowing query params */
  chatRootOrRoomWithQuery: /\/chat\/-(?:\/(?!manage(?:\?|$))[a-zA-Z0-9]+)?(?:\?.*)?$/,
  /** Any room with query params (e.g. ?highlight=) */
  anyRoomWithQuery: /\/chat\/-\/(?!manage(?:\/|\?|$))[a-zA-Z0-9]+/,
  /** Email verified redirect */
  emailVerified: /\?email_verified=true/
};

// --- Remote instance helper ---

/**
 * Build a route for a remote instance (used in multi-instance tests).
 * Unlike the home instance routes above, these use a hostname segment instead of "-".
 */
export const remote = {
  room: (hostname: string, roomId: string) => `/chat/${hostname}/${roomId}`
};
