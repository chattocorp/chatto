import { roomSidebarWidthSlot } from '$lib/storage/roomSidebarWidth';
import { createPaneWidthState } from '$lib/state/paneWidth.svelte';

export const roomSidebarWidth = createPaneWidthState(roomSidebarWidthSlot);
