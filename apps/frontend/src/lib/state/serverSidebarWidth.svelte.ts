import { serverSidebarWidthSlot } from '$lib/storage/serverSidebarWidth';
import { createPaneWidthState } from '$lib/state/paneWidth.svelte';

export const serverSidebarWidth = createPaneWidthState(serverSidebarWidthSlot);
