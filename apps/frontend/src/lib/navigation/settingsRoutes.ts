/** The canonical unified Settings entry-point route. */
export const SERVER_SETTINGS_ROOT_ROUTE = '/chat/[serverId]/settings';

/** The first page in the unified Settings navigation. */
export const DEFAULT_SETTINGS_PAGE = 'appearance';

/** The route for the first page in the unified Settings navigation. */
export const DEFAULT_SERVER_SETTINGS_ROUTE =
	`${SERVER_SETTINGS_ROOT_ROUTE}/${DEFAULT_SETTINGS_PAGE}` as const;
