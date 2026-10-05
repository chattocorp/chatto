const HIGHLIGHT_DELAY_MS = 150;
const LONG_PRESS_MS = 500;

/**
 * The touch long-press gesture of one message row.
 *
 * After a short delay the row highlights; after the full delay the gesture calls
 * `onLongPress`, which opens the touch action sheet. The release of that press can
 * produce a click on an action beneath the finger, so the gesture discards that click.
 */
export class MessageLongPressGesture {
  /** True while the row shows the long-press highlight. */
  active = $state(false);

  readonly #onLongPress: () => void;
  #highlightTimer: ReturnType<typeof setTimeout> | null = null;
  #longPressTimer: ReturnType<typeof setTimeout> | null = null;
  #openingClickTimer: ReturnType<typeof setTimeout> | null = null;
  /** True from the moment the gesture fires until the press ends. */
  #fired = false;
  #handlePressEnd = () => this.finish();
  #discardOpeningClick = (event: MouseEvent) => {
    this.#clearOpeningClickGuard();
    if (event.detail === 0) return;
    event.preventDefault();
    event.stopImmediatePropagation();
  };
  #allowNewPress = () => this.#clearOpeningClickGuard();

  constructor(onLongPress: () => void) {
    this.#onLongPress = onLongPress;
  }

  /** True while a press can still open the action sheet or has just opened it. */
  get pending(): boolean {
    return (
      this.#highlightTimer !== null || this.#longPressTimer !== null || this.active || this.#fired
    );
  }

  start(): void {
    this.cancel();
    window.addEventListener('touchend', this.#handlePressEnd, true);
    window.addEventListener('touchcancel', this.#handlePressEnd, true);
    window.addEventListener('pointerup', this.#handlePressEnd, true);
    window.addEventListener('pointercancel', this.#handlePressEnd, true);
    window.addEventListener('mouseup', this.#handlePressEnd, true);
    this.#highlightTimer = setTimeout(() => {
      this.active = true;
    }, HIGHLIGHT_DELAY_MS);
    this.#longPressTimer = setTimeout(() => {
      this.#longPressTimer = null;
      this.#fired = true;
      this.active = false;
      this.#onLongPress();
      // A new press clears the guard, so immediate intentional taps still work.
      this.#guardOpeningClick();
    }, LONG_PRESS_MS);
  }

  /** Stop tracking the press even when its release misses the message. */
  finish(): void {
    this.#fired = false;
    this.cancel();
  }

  /** Cancels a press that has not fired yet, for example when the finger moves. */
  cancel(): void {
    this.active = false;
    if (this.#highlightTimer) {
      clearTimeout(this.#highlightTimer);
      this.#highlightTimer = null;
    }
    if (this.#longPressTimer) {
      clearTimeout(this.#longPressTimer);
      this.#longPressTimer = null;
    }
    if (!this.#fired) this.#stopListeningForPressEnd();
  }

  /**
   * Stops the gesture when its row unmounts. The opening-click guard stays: the row can
   * unmount while the finger is still down, and the guard removes itself.
   */
  dispose(): void {
    this.finish();
  }

  #stopListeningForPressEnd(): void {
    window.removeEventListener('touchend', this.#handlePressEnd, true);
    window.removeEventListener('touchcancel', this.#handlePressEnd, true);
    window.removeEventListener('pointerup', this.#handlePressEnd, true);
    window.removeEventListener('pointercancel', this.#handlePressEnd, true);
    window.removeEventListener('mouseup', this.#handlePressEnd, true);
  }

  #clearOpeningClickGuard(): void {
    window.removeEventListener('click', this.#discardOpeningClick, true);
    window.removeEventListener('pointerdown', this.#allowNewPress, true);
    window.removeEventListener('touchstart', this.#allowNewPress, true);
    if (this.#openingClickTimer) clearTimeout(this.#openingClickTimer);
    this.#openingClickTimer = null;
  }

  #guardOpeningClick(): void {
    this.#clearOpeningClickGuard();
    window.addEventListener('click', this.#discardOpeningClick, true);
    window.addEventListener('pointerdown', this.#allowNewPress, true);
    window.addEventListener('touchstart', this.#allowNewPress, true);
    this.#openingClickTimer = setTimeout(() => this.#clearOpeningClickGuard(), 2000);
  }
}
