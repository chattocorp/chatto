/** Owns temporary copy labels and timers for one component's buttons. */
export class CopyFeedback {
  #pending = new Map<HTMLButtonElement, () => void>();

  /** Call only after the clipboard write succeeds. Repeated copies restart the cue. */
  show(button: HTMLButtonElement, label: string): void {
    this.#pending.get(button)?.();
    const previousLabel = button.getAttribute('aria-label');
    const previousTitle = button.getAttribute('title');
    const restore = () => {
      clearTimeout(timer);
      delete button.dataset.copied;
      if (previousLabel === null) button.removeAttribute('aria-label');
      else button.setAttribute('aria-label', previousLabel);
      if (previousTitle === null) button.removeAttribute('title');
      else button.setAttribute('title', previousTitle);
      this.#pending.delete(button);
    };
    const timer = setTimeout(restore, 1400);
    this.#pending.set(button, restore);
    button.dataset.copied = 'true';
    button.setAttribute('aria-label', label);
    button.title = label;
  }

  /** Restore labels and release detached buttons when the owner unmounts. */
  dispose(): void {
    for (const restore of this.#pending.values()) restore();
  }
}
