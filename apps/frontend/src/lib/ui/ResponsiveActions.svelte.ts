import { tick, untrack } from 'svelte';
import type { Attachment } from 'svelte/attachments';

/** Host callbacks keep the action layout independent of menu content and domain state. */
export type ResponsiveActionsOptions = {
  /** Elements below this width in root em units use overflow actions. */
  breakpointRem: number;
  /** Whether this element owns the host's currently open menu. */
  isMenuOpen: () => boolean;
  /** Dismiss the host's menu when the layout changes or its element disappears. */
  dismissMenu: () => void;
  /** A connected replacement control when the original element disappears. */
  focusFallback?: () => HTMLElement | null | undefined;
  /** Preserve focus explicitly moved to another control in this host. */
  focusScope?: () => HTMLElement | null | undefined;
};

/**
 * Owns responsive action layout for one mounted element. Hosts render inline
 * or overflow snippets from `compact`, and keep action state outside this class.
 * Attach `observe`, `inline`, and `trigger` to the container, inline region,
 * and overflow button. Menu close callbacks can call `restoreFocus`.
 */
export class ResponsiveActions {
  /** Use overflow until the mounted element's width is known. */
  compact = $state(true);
  #element: HTMLElement | null = null;
  #inline: HTMLElement | null = null;
  #trigger: HTMLElement | null = null;
  #options: ResponsiveActionsOptions;

  constructor(options: ResponsiveActionsOptions) {
    this.#options = options;
  }

  /** Mounted container; also available to host actions such as fullscreen. */
  get element(): HTMLElement | null {
    return this.#element;
  }

  /** Observe border-box width without including transforms or viewport width. */
  observe: Attachment<HTMLElement> = (element) =>
    untrack(() => {
      this.#element = element;
      const boxExtraWidth = () => {
        const style = getComputedStyle(element);
        return (
          parseFloat(style.paddingLeft) +
          parseFloat(style.paddingRight) +
          parseFloat(style.borderLeftWidth) +
          parseFloat(style.borderRightWidth)
        );
      };
      const update = (width: number) => {
        const rootEm = parseFloat(getComputedStyle(document.documentElement).fontSize);
        const compact = width < this.#options.breakpointRem * rootEm;
        if (compact === this.compact) return;
        const ownsMenu = this.#options.isMenuOpen();
        const focusedInline = this.#inline?.contains(document.activeElement);
        this.compact = compact;
        if (ownsMenu) this.#options.dismissMenu();
        if (ownsMenu || focusedInline) void this.restoreFocus();
      };
      const style = getComputedStyle(element);
      update(parseFloat(style.width) + (style.boxSizing === 'border-box' ? 0 : boxExtraWidth()));
      const observer = new ResizeObserver(([entry]) => {
        update(entry.borderBoxSize[0]?.inlineSize ?? entry.contentRect.width + boxExtraWidth());
      });
      observer.observe(element, { box: 'border-box' });
      return () => {
        observer.disconnect();
        if (this.#options.isMenuOpen()) {
          this.#options.dismissMenu();
          void this.restoreFocus();
        }
      };
    });

  /** Mark the inline action region so focus can survive its removal on resize. */
  inline: Attachment<HTMLElement> = (element) => {
    this.#inline = element;
    return () => {
      if (this.#inline === element) this.#inline = null;
    };
  };

  /** Register the overflow button without relying on host selectors or labels. */
  trigger: Attachment<HTMLElement> = (element) => {
    // Retain the reference through teardown so delayed dismissal can detect removal.
    this.#trigger = element;
  };

  /** Wait for layout changes, then focus the trigger or the host's replacement control. */
  restoreFocus = async (): Promise<void> => {
    await tick();
    const focused = document.activeElement;
    if (
      focused instanceof HTMLElement &&
      focused.isConnected &&
      !this.#element?.contains(focused) &&
      this.#options.focusScope?.()?.contains(focused)
    )
      return;
    const target = this.#trigger?.isConnected ? this.#trigger : this.#options.focusFallback?.();
    target?.focus();
  };
}
