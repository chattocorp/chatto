import type { LayoutLoad } from './$types';

/**
 * Expose the selected room to the room layout.
 *
 * This load owns the room param so that a room switch re-runs only this load,
 * not the server layout's access checks and saved-view restore.
 */
export const load: LayoutLoad = ({ params }) => ({ roomId: params.roomId });
