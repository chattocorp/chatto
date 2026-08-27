import { threadPaneWidthSlot } from '$lib/storage/threadPaneWidth';
import { createPaneWidthState } from '$lib/state/paneWidth.svelte';

export const threadPaneWidth = createPaneWidthState(threadPaneWidthSlot);
