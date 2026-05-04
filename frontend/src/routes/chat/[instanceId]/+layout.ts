import { redirect } from '@sveltejs/kit';
import type { LayoutLoad } from './$types';

/**
 * Detects the legacy URL shape `/chat/<instanceSeg>/<spaceId>/<rest>` (where
 * spaceId looks like `S` + 14 nanoid chars) and redirects to the collapsed
 * shape `/chat/<instanceSeg>/<rest>` introduced in ADR-027 / #330. Lets old
 * bookmarks and external links keep working through the rename.
 */
const LEGACY_SPACE_ID_RE = /^S[0-9A-Za-z]{14}$/;

export const load: LayoutLoad = ({ url, params }) => {
	const segments = url.pathname.split('/');
	// segments: ['', 'chat', '<instanceSeg>', '<maybeSpaceId>', ...]
	const candidate = segments[3];
	if (candidate && LEGACY_SPACE_ID_RE.test(candidate)) {
		const collapsed = [...segments.slice(0, 3), ...segments.slice(4)].join('/');
		throw redirect(307, collapsed + url.search);
	}

	// Instance validation happens in +layout.svelte (after ensureHome() has run).
	// Load functions run before component scripts, so the registry isn't populated yet.
	return {
		instanceSegment: params.instanceId,

		/** The currently active room (from child route params). */
		roomId: params.roomId
	};
};
