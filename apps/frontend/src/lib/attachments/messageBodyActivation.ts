import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';

const INTERACTIVE =
  'a, button, input, textarea, select, summary, audio, video, [contenteditable]:not([contenteditable="false"]), [role="button"], [role="link"], [tabindex], .mention, .message-timestamp';

/**
 * Add a pointer shortcut to selectable message text. The existing thread
 * controls remain the keyboard interface. Never cancel native pointer events.
 * Recreate this attachment when a virtualized row changes message identity.
 */
export function messageBodyActivation(activate?: () => void): Attachment<HTMLElement> {
  return (element) => {
    if (!activate) return;
    let press:
      { x: number; y: number; time: number; pointerType: string; blocked: boolean } | undefined;
    let selectionFrame: number | undefined;
    const cancel = () => {
      press = undefined;
      if (selectionFrame !== undefined) cancelAnimationFrame(selectionFrame);
      selectionFrame = undefined;
    };
    const hasSelection = () => {
      const selection = window.getSelection();
      if (!selection || selection.isCollapsed) return false;
      for (let i = 0; i < selection.rangeCount; i++) {
        if (selection.getRangeAt(i).intersectsNode(element)) return true;
      }
      return false;
    };
    const interactive = (target: EventTarget | null) => {
      const control = target instanceof Element ? target.closest(INTERACTIVE) : null;
      // Only controls inside this body own the click. The timeline itself is
      // keyboard-focusable, but its tabindex must not disable every message.
      return control !== null && element.contains(control);
    };
    const dispose = [
      on(window, 'pointerdown', cancel, { capture: true }),
      on(element, 'pointerdown', (event) => {
        press = {
          x: event.clientX,
          y: event.clientY,
          time: performance.now(),
          pointerType: event.pointerType,
          // A normal click can clear an old selection. Check the resulting
          // selection at click time, not before the browser handles the press.
          blocked: event.button !== 0 || !event.isPrimary || interactive(event.target)
        };
      }),
      on(window, 'pointermove', (event) => {
        if (press && Math.hypot(event.clientX - press.x, event.clientY - press.y) > 6) {
          press.blocked = true;
        }
      }),
      on(element, 'pointercancel', cancel),
      on(element, 'contextmenu', () => {
        if (press) press.blocked = true;
      }),
      on(window, 'blur', cancel),
      on(element, 'click', (event) => {
        const gesture = press;
        press = undefined;
        if (
          !gesture ||
          gesture.blocked ||
          event.defaultPrevented ||
          event.button !== 0 ||
          event.detail > 1 ||
          event.ctrlKey ||
          event.metaKey ||
          event.altKey ||
          event.shiftKey ||
          interactive(event.target)
        )
          return;
        // A long press belongs to the action sheet or native text selection.
        if (gesture.pointerType === 'touch' && performance.now() - gesture.time >= 500) return;
        if (hasSelection()) {
          // Chromium can clear an old selection in the click's default action,
          // after this handler. Check once before the next paint. Do not add a
          // double-click timeout or delay ordinary unselected clicks.
          selectionFrame = requestAnimationFrame(() => {
            selectionFrame = undefined;
            if (element.isConnected && !hasSelection()) activate();
          });
          return;
        }
        activate();
      })
    ];
    return () => {
      cancel();
      for (const stop of dispose) stop();
    };
  };
}
