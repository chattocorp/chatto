import type { LayoutLoad } from './$types';

export const load: LayoutLoad = ({ params }) => {
	// Server validation happens in +layout.svelte (after ensureHome() has run).
	// Load functions run before component scripts, so the registry isn't populated yet.
	return {
		serverSegment: params.serverId,

		/** The currently active room (from child route params). */
		roomId: params.roomId
	};
};
