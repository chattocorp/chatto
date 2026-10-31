import type { LayoutLoad } from './$types';
import { loadEditor } from '$lib/components/composer/loadEditor';
import { userPreferences } from '$lib/state/userPreferences.svelte';

/**
 * Expose the selected room to the room layout.
 *
 * This load owns the room param, so a room switch does not re-run the server
 * layout's access checks because of this param.
 */
export const load: LayoutLoad = ({ params }) => {
  // Overlap the selected editor download with room startup. Rendering can retry a failed import.
  void Promise.resolve(loadEditor(userPreferences.composerEditor)).catch(() => {});
  return { roomId: params.roomId };
};
