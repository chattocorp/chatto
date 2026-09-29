/**
 * Browser page lifecycle tracking, shared by every connected server.
 *
 * Outside a browser, the app counts as focused and visible, and nothing
 * changes.
 */

import { batch, signal } from '@chatto/client/reactivity';

/** Page lifecycle state: focus, visibility, foreground returns, and network recovery. */
export class AppState {
  readonly #isFocusedSignal = signal(typeof document !== 'undefined' ? document.hasFocus() : true);
  get isFocused() {
    return this.#isFocusedSignal.get();
  }
  set isFocused(value) {
    this.#isFocusedSignal.set(value);
  }
  readonly #isVisibleSignal = signal(
    typeof document !== 'undefined' ? document.visibilityState === 'visible' : true
  );
  get isVisible() {
    return this.#isVisibleSignal.get();
  }
  set isVisible(value) {
    this.#isVisibleSignal.set(value);
  }
  readonly #foregroundRevisionSignal = signal(0);
  get foregroundRevision() {
    return this.#foregroundRevisionSignal.get();
  }
  set foregroundRevision(value) {
    this.#foregroundRevisionSignal.set(value);
  }
  readonly #onlineRevisionSignal = signal(0);
  get onlineRevision() {
    return this.#onlineRevisionSignal.get();
  }
  set onlineRevision(value) {
    this.#onlineRevisionSignal.set(value);
  }

  private foregroundActive =
    typeof document !== 'undefined' ? document.visibilityState === 'visible' : true;

  /**
   * True when the app is visible and focused. Continuous message arrivals
   * use this stricter state. Target entry and foreground activation only need
   * visibility because some mobile app resumes do not restore focus events.
   */
  get isPresent(): boolean {
    return this.isFocused && this.isVisible;
  }

  constructor() {
    if (typeof window !== 'undefined') {
      window.addEventListener('focus', () => {
        this.isFocused = true;
      });
      window.addEventListener('blur', () => {
        this.isFocused = false;
      });
      window.addEventListener('pageshow', () => {
        this.reconcileVisibility();
      });
      window.addEventListener('pagehide', () => {
        this.markBackgrounded();
      });
      window.addEventListener('online', () => {
        this.onlineRevision += 1;
      });
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', () => {
        this.reconcileVisibility();
      });
      document.addEventListener('freeze', () => {
        this.markBackgrounded();
      });
      document.addEventListener('resume', () => {
        this.reconcileVisibility();
      });
      document.addEventListener(
        'pointerdown',
        (event) => {
          if (event.isTrusted) this.activateFromInteraction();
        },
        { capture: true }
      );
      document.addEventListener(
        'keydown',
        (event) => {
          if (event.isTrusted) this.activateFromInteraction();
        },
        { capture: true }
      );
    }
  }

  private markBackgrounded() {
    // Observers see all lifecycle fields change together.
    batch(() => {
      this.foregroundActive = false;
      this.isFocused = false;
      this.isVisible = false;
    });
  }

  private activateFromInteraction() {
    // Observers see all lifecycle fields change together.
    batch(() => {
      this.isFocused = true;
      this.isVisible = true;
      if (!this.foregroundActive) {
        this.foregroundActive = true;
        this.foregroundRevision += 1;
      }
    });
  }

  private reconcileVisibility() {
    // Observers see all lifecycle fields change together.
    batch(() => {
      const visible = document.visibilityState === 'visible';
      this.isVisible = visible;

      if (!visible) {
        this.foregroundActive = false;
        return;
      }

      this.isFocused = document.hasFocus();
      if (!this.foregroundActive) {
        this.foregroundActive = true;
        this.foregroundRevision += 1;
      }
    });
  }
}

export const appState = new AppState();
