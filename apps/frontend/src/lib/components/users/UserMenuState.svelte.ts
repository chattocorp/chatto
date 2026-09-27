import {
  contextMenuTrigger,
  type ContextMenuTriggerDetails
} from '$lib/ui/contextMenuTrigger.svelte';

type MenuSelection<T> = {
  target: T;
  anchorRect?: { top: number; bottom: number; left: number };
  position?: ContextMenuTriggerDetails['position'];
  presentation: ContextMenuTriggerDetails['presentation'];
};

/**
 * Owns one user menu per host. Targets can be IDs resolved from live state or
 * include host-specific context, such as the selected call audio source.
 * The host owns permissions, user data, and actions; closing clears the target.
 */
export class UserMenuState<T> {
  #selection = $state.raw<MenuSelection<T> | null>(null);

  constructor(private readonly onopen?: () => void) {}

  /** Current target and placement, or null when the menu is closed. */
  get selection() {
    return this.#selection;
  }

  /** Selected host target. Null is reserved for a closed menu. */
  get target(): T | null {
    return this.#selection?.target ?? null;
  }

  /** Open at a button or other clicked element. */
  open(target: T, event: MouseEvent): void {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    this.onopen?.();
    this.#selection = {
      target,
      anchorRect: { top: rect.top, bottom: rect.bottom, left: rect.left },
      presentation: 'auto'
    };
  }

  /** Toggle a button menu for the same target; ID targets use value equality. */
  toggle(target: T, event: MouseEvent): void {
    if (this.target === target) this.close();
    else this.open(target, event);
  }

  /** Resolve the target when a right-click or touch long-press occurs. */
  trigger(getTarget: () => T | null) {
    return contextMenuTrigger((details) => {
      const target = getTarget();
      if (target === null) return;
      this.onopen?.();
      this.#selection = { target, ...details };
    });
  }

  /** Bound callback for dismissal and host actions that replace the menu. */
  close = (): void => {
    this.#selection = null;
  };
}
