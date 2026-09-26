import type { LayoutLoad } from './$types';

/**
 * Expose the selected room to the room layout.
 *
 * This load owns the room param, so a room switch does not re-run the server
 * layout's access checks because of this param.
 */
export const load: LayoutLoad = ({ params }) => ({ roomId: params.roomId });
