import type { RoomEventView } from '$lib/render/types';

/** Reactive test double for exercising thread data that resolves after mount. */
export class ThreadPaneTestStore {
  threadEvents = $state<RoomEventView[]>([]);
  isInitialLoading = $state(false);
}
