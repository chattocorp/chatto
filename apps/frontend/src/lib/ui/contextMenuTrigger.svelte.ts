import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';

const LONG_PRESS_MS = 500;
const LONG_PRESS_CANCEL_PX = 8;

export type ContextMenuTriggerDetails = {
  position: { x: number; y: number };
  presentation: 'auto' | 'sheet';
};

/**
 * Opens a context menu for a native context-menu gesture or a stationary touch long-press.
 * Movement cancels the pending touch gesture so normal sidebar scrolling remains native.
 * `touchOnly` leaves desktop context menus with the element's existing owner.
 */
export function contextMenuTrigger(
  onopen: (details: ContextMenuTriggerDetails) => void,
  options: { touchOnly?: boolean } = {}
): Attachment<HTMLElement> {
  return (node) => {
    let timer: number | null = null;
    let suppressClickTimer: number | null = null;
    let pointerId: number | null = null;
    let touchSequence = false;
    let startX = 0;
    let startY = 0;
    let suppressClick = false;
    let openedDuringTouch = false;

    function clearClickSuppression(): void {
      suppressClick = false;
      if (suppressClickTimer !== null) window.clearTimeout(suppressClickTimer);
      suppressClickTimer = null;
      window.removeEventListener('click', discardOpeningClick, true);
      window.removeEventListener('pointerdown', clearClickSuppression, true);
      window.removeEventListener('keydown', clearClickSuppression, true);
    }

    function discardOpeningClick(event: MouseEvent): void {
      clearClickSuppression();
      event.preventDefault();
      event.stopImmediatePropagation();
    }

    function cancelLongPress(): void {
      if (timer !== null) window.clearTimeout(timer);
      timer = null;
      pointerId = null;
    }

    function armClickSuppression(): void {
      suppressClick = true;
      // A modal can retarget the release click onto its content. Guard the
      // window, not just the trigger. A new press or key is deliberate input.
      window.addEventListener('click', discardOpeningClick, true);
      window.addEventListener('pointerdown', clearClickSuppression, true);
      window.addEventListener('keydown', clearClickSuppression, true);
      if (suppressClickTimer !== null) window.clearTimeout(suppressClickTimer);
      // Some browsers omit the synthetic click after a native long-press menu.
      // Do not let that suppress a later deliberate click.
      suppressClickTimer = window.setTimeout(clearClickSuppression, 1000);
    }

    function finishTouch(): void {
      // A held finger can outlast the fallback timer. Suppress the release
      // click even when the user keeps holding after the sheet opens.
      if (openedDuringTouch) armClickSuppression();
      openedDuringTouch = false;
      cancelLongPress();
      touchSequence = false;
    }

    const cleanups = [
      on(node, 'contextmenu', (event) => {
        const fromTouch = touchSequence || openedDuringTouch || suppressClick;
        if (options.touchOnly && !fromTouch) return;
        event.preventDefault();
        if (options.touchOnly) event.stopPropagation();
        if (fromTouch && (openedDuringTouch || suppressClick)) return;
        cancelLongPress();
        if (fromTouch) {
          openedDuringTouch = true;
          armClickSuppression();
        }
        onopen({
          position: { x: event.clientX, y: event.clientY },
          presentation: fromTouch ? 'sheet' : 'auto'
        });
      }),
      on(node, 'pointerdown', (event) => {
        if (event.pointerType !== 'touch' || !event.isPrimary) return;
        cancelLongPress();
        touchSequence = true;
        openedDuringTouch = false;
        pointerId = event.pointerId;
        startX = event.clientX;
        startY = event.clientY;
        timer = window.setTimeout(() => {
          timer = null;
          pointerId = null;
          openedDuringTouch = true;
          armClickSuppression();
          onopen({
            position: { x: startX, y: startY },
            presentation: 'sheet'
          });
        }, LONG_PRESS_MS);
      }),
      on(node, 'pointermove', (event) => {
        if (event.pointerId !== pointerId) return;
        if (
          Math.abs(event.clientX - startX) >= LONG_PRESS_CANCEL_PX ||
          Math.abs(event.clientY - startY) >= LONG_PRESS_CANCEL_PX
        ) {
          cancelLongPress();
          touchSequence = false;
        }
      }),
      on(node, 'pointerup', finishTouch),
      on(node, 'pointercancel', finishTouch)
    ];

    return () => {
      cancelLongPress();
      clearClickSuppression();
      for (const cleanup of cleanups) cleanup();
    };
  };
}
