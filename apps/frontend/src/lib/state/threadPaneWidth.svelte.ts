import {
  getThreadPaneWidth,
  setThreadPaneWidth,
  THREAD_PANE_DEFAULT_WIDTH,
  THREAD_PANE_MAX_WIDTH,
  THREAD_PANE_MIN_WIDTH
} from '$lib/storage/threadPaneWidth';

class ThreadPaneWidthState {
  #width = $state(getThreadPaneWidth());

  get value(): number {
    return this.#width;
  }

  set(width: number): void {
    const clamped = Math.min(THREAD_PANE_MAX_WIDTH, Math.max(THREAD_PANE_MIN_WIDTH, width));
    this.#width = clamped;
    setThreadPaneWidth(clamped);
  }

  reset(): void {
    this.set(THREAD_PANE_DEFAULT_WIDTH);
  }
}

export const threadPaneWidth = new ThreadPaneWidthState();
