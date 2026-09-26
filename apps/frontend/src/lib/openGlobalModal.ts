import { pushState } from '$app/navigation';
import { page } from '$app/state';
import { frameViewInState, isFrameViewModal, type ChatModal } from '$lib/modal';

/**
 * Open a global modal in a new history entry. A dialog that opens above a
 * frame view keeps the view in its entry, so the view stays mounted and keeps
 * its state until the user closes the view.
 */
export function openGlobalModal(modal: ChatModal): void {
  const frameView = isFrameViewModal(modal) ? undefined : frameViewInState(page.state);
  pushState('', frameView ? { modal, frameView } : { modal });
}
