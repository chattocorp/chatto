import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
/** Reactive test double for exercising thread data that resolves after mount. */
export class ThreadPaneTestStore {
  threadEvents = $state<TimelineEventView[]>([]);
  isInitialLoading = $state(false);
}
