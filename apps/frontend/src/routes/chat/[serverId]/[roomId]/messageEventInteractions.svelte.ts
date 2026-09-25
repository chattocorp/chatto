const HIGHLIGHT_DELAY_MS = 150;
const LONG_PRESS_MS = 500;

export type MessageContextMenuPosition = {
  x: number;
  y: number;
  alignRight?: boolean;
  centerX?: boolean;
};

export class MessageEventInteractionState {
  showActionSheet = $state(false);
  longPressActive = $state(false);
  contextMenuPosition = $state<MessageContextMenuPosition | null>(null);
  emojiPickerPosition = $state<{ x: number; y: number } | null>(null);
  emojiPickerPresentation = $state<'auto' | 'sheet'>('auto');

  #highlightTimer: ReturnType<typeof setTimeout> | null = null;
  #longPressTimer: ReturnType<typeof setTimeout> | null = null;
  #openingClickTimer: ReturnType<typeof setTimeout> | null = null;
  #handlePressEnd = () => this.finishLongPress();
  #discardOpeningClick = (event: MouseEvent) => {
    this.#clearOpeningClickGuard();
    if (event.detail === 0) return;
    event.preventDefault();
    event.stopImmediatePropagation();
  };
  #allowNewPress = () => this.#clearOpeningClickGuard();

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

  get hasOpenActionSurface(): boolean {
    return this.showActionSheet || this.contextMenuPosition !== null;
  }

  get hasActiveLongPressGesture(): boolean {
    return (
      this.#highlightTimer !== null ||
      this.#longPressTimer !== null ||
      this.longPressActive ||
      this.showActionSheet
    );
  }

  get forceHoverActionsVisible(): boolean {
    return this.emojiPickerPosition !== null || this.contextMenuPosition !== null;
  }

  openContextMenuFromToolbar(event: MouseEvent): void {
    const button = event.currentTarget as HTMLElement;
    const toolbar = button.closest('[role="toolbar"]') as HTMLElement | null;
    const rect = toolbar?.getBoundingClientRect() ?? button.getBoundingClientRect();
    this.contextMenuPosition = { x: rect.right, y: rect.top, alignRight: true };
  }

  openContextMenuAtPointer(event: MouseEvent): void {
    this.contextMenuPosition = { x: event.clientX, y: event.clientY };
  }

  closeContextMenu(): void {
    this.contextMenuPosition = null;
  }

  openEmojiPicker(presentation: 'auto' | 'sheet' = 'auto'): void {
    this.emojiPickerPresentation = presentation;
    this.emojiPickerPosition = this.contextMenuPosition ?? { x: 0, y: 0 };
  }

  openEmojiPickerFromEvent(event: MouseEvent): void {
    this.emojiPickerPresentation = 'auto';
    this.emojiPickerPosition = { x: event.clientX, y: event.clientY };
  }

  openEmojiPickerFromToolbar(event: MouseEvent): void {
    this.emojiPickerPresentation = 'auto';
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    this.emojiPickerPosition = { x: rect.left, y: rect.bottom + 4 };
  }

  closeEmojiPicker(): void {
    this.emojiPickerPresentation = 'auto';
    this.emojiPickerPosition = null;
  }

  startLongPress(): void {
    if (this.showActionSheet) return;
    this.cancelLongPress();
    window.addEventListener('touchend', this.#handlePressEnd, true);
    window.addEventListener('touchcancel', this.#handlePressEnd, true);
    window.addEventListener('pointerup', this.#handlePressEnd, true);
    window.addEventListener('pointercancel', this.#handlePressEnd, true);
    window.addEventListener('mouseup', this.#handlePressEnd, true);
    this.#highlightTimer = setTimeout(() => {
      this.longPressActive = true;
    }, HIGHLIGHT_DELAY_MS);
    this.#longPressTimer = setTimeout(() => {
      this.showActionSheet = true;
      this.longPressActive = false;
      // The opening release can produce a click on an action beneath the finger.
      // A new press clears the guard, so immediate intentional taps still work.
      this.#guardOpeningClick();
    }, LONG_PRESS_MS);
  }

  /** Stop tracking the press even when its release misses the message. */
  finishLongPress(): void {
    this.cancelLongPress();
    this.#stopListeningForPressEnd();
  }

  cancelLongPress(): void {
    if (this.longPressActive) {
      this.longPressActive = false;
    }
    if (this.#highlightTimer) {
      clearTimeout(this.#highlightTimer);
      this.#highlightTimer = null;
    }
    if (this.#longPressTimer) {
      clearTimeout(this.#longPressTimer);
      this.#longPressTimer = null;
    }
    if (!this.showActionSheet) this.#stopListeningForPressEnd();
  }

  closeActionSheet(): void {
    this.showActionSheet = false;
    this.#stopListeningForPressEnd();
    this.#clearOpeningClickGuard();
  }

  dispose(): void {
    this.cancelLongPress();
    this.#stopListeningForPressEnd();
    this.#clearOpeningClickGuard();
  }
}
