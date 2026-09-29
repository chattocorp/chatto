/**
 * Navigation state of a timeline that jumped to an older message, outside the
 * latest window. A room view owns one instance and drives the timeline's
 * `MessagesStore` through {@link JumpToMessageState.show},
 * {@link JumpToMessageState.loadNewer}, and
 * {@link JumpToMessageState.returnToLatest}.
 */

import { batch, signal } from '@chatto/client/reactivity';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { clearTimelineViewport } from './timelineViewport';

export class JumpToMessageState {
  readonly #isJumpedModeSignal = signal(false);
  get isJumpedMode() {
    return this.#isJumpedModeSignal.get();
  }
  set isJumpedMode(value) {
    this.#isJumpedModeSignal.set(value);
  }
  readonly #scrollToEventIdSignal = signal<string | null>(null);
  get scrollToEventId(): string | null {
    return this.#scrollToEventIdSignal.get();
  }
  set scrollToEventId(value: string | null) {
    this.#scrollToEventIdSignal.set(value);
  }
  readonly #hasReachedEndSignal = signal(false);
  get hasReachedEnd() {
    return this.#hasReachedEndSignal.get();
  }
  set hasReachedEnd(value) {
    this.#hasReachedEndSignal.set(value);
  }
  readonly #hasOlderMessagesSignal = signal(false);
  get hasOlderMessages() {
    return this.#hasOlderMessagesSignal.get();
  }
  set hasOlderMessages(value) {
    this.#hasOlderMessagesSignal.set(value);
  }
  readonly #isLoadingNewerSignal = signal(false);
  get isLoadingNewer() {
    return this.#isLoadingNewerSignal.get();
  }
  set isLoadingNewer(value) {
    this.#isLoadingNewerSignal.set(value);
  }

  private _jumpFn: ((eventId: string) => Promise<boolean>) | null = null;

  setJumpHandler(fn: (eventId: string) => Promise<boolean>) {
    this._jumpFn = fn;
  }

  async jumpToMessage(eventId: string): Promise<boolean> {
    if (this._jumpFn) {
      return this._jumpFn(eventId);
    }
    return false;
  }

  /**
   * Show a message in `store`, replacing its window when needed, and enter
   * jumped mode when newer events follow the new window. Returns false when
   * the message cannot be shown.
   */
  async show(store: MessagesStore, eventId: string): Promise<boolean> {
    // A replaced window cannot release a newer-page flag of the old one.
    if (!store.getEventById(eventId)) this.isLoadingNewer = false;
    const result = await store.jumpToMessage(eventId);
    switch (result.status) {
      case 'shown':
        this.scrollToEventId = eventId;
        return true;
      case 'loaded':
        batch(() => {
          // Only enter jumped mode when newer messages exist beyond this window.
          this.isJumpedMode = result.hasNewer;
          this.hasReachedEnd = !result.hasNewer;
          this.hasOlderMessages = result.hasOlder;
          this.scrollToEventId = eventId;
        });
        return true;
      case 'missing':
        batch(() => {
          this.scrollToEventId = null;
          this.isJumpedMode = false;
          this.hasReachedEnd = false;
          this.hasOlderMessages = false;
        });
        return false;
      case 'superseded':
        return false;
    }
  }

  /** Append the next newer page while in jumped mode. */
  async loadNewer(store: MessagesStore): Promise<void> {
    if (this.isLoadingNewer || this.hasReachedEnd || !store.canLoadNewer) return;
    this.isLoadingNewer = true;
    // The user can leave jumped mode while the page is in flight.
    const result = await store.loadNewer(() => this.isJumpedMode);
    // A replacement window owns the flag now.
    if (result.status === 'superseded') return;
    batch(() => {
      if (result.status === 'loaded' && !result.hasNewer) this.hasReachedEnd = true;
      this.isLoadingNewer = false;
    });
  }

  /** Leave jumped mode and load the latest window of `store`. */
  returnToLatest(store: MessagesStore): Promise<boolean> {
    clearTimelineViewport(store);
    this.reset();
    return store.jumpToLatest();
  }

  reset(): void {
    batch(() => {
      this.isJumpedMode = false;
      this.scrollToEventId = null;
      this.hasReachedEnd = false;
      this.hasOlderMessages = false;
      this.isLoadingNewer = false;
    });
  }
}
