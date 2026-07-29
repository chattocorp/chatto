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

  get hasOpenActionSurface(): boolean {
    return this.showActionSheet || this.contextMenuPosition !== null;
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
    this.cancelLongPress();
    this.#highlightTimer = setTimeout(() => {
      this.longPressActive = true;
    }, HIGHLIGHT_DELAY_MS);
    this.#longPressTimer = setTimeout(() => {
      this.showActionSheet = true;
      this.longPressActive = false;
    }, LONG_PRESS_MS);
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
  }

  closeActionSheet(): void {
    this.showActionSheet = false;
  }

  dispose(): void {
    this.cancelLongPress();
  }
}
