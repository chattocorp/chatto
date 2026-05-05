import type { LayoutLoad } from './$types';

export const load: LayoutLoad = ({ params }) => {
	// Instance validation happens in +layout.svelte (after ensureHome() has run).
	// Load functions run before component scripts, so the registry isn't populated yet.
	return {
		instanceSegment: params.instanceId,

		/** The currently active room (from child route params). */
		roomId: params.roomId
	};
};
