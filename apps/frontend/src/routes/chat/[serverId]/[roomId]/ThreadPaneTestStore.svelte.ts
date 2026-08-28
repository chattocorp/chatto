import type { TimelineEventView } from '$lib/render/timelineEvents';
/** Reactive test double for exercising thread data that resolves after mount. */
export class ThreadPaneTestStore {
  threadEvents = $state<TimelineEventView[]>([]);
  isInitialLoading = $state(false);
}
