/**
 * Navigation state of a timeline that jumped to an older message, outside the
 * latest window. A room view owns one instance and passes it to
 * `MessagesStore` jump and load operations.
 */

import { signal } from '../../reactivity/index.js';

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

  reset(): void {
    this.isJumpedMode = false;
    this.scrollToEventId = null;
    this.hasReachedEnd = false;
    this.hasOlderMessages = false;
    this.isLoadingNewer = false;
  }
}
