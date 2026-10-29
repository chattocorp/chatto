import { on } from 'svelte/events';
import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';

/** Own one palette opening. Narrow panes leave the app header interactive;
 * desktop palettes use native modal focus containment. Resizing keeps the same
 * content and focused field, and closing restores the original opener.
 */
export function paletteDialog(node: HTMLDialogElement, visible: boolean, onclose: () => void) {
  if (!visible) return;
  const narrow = window.matchMedia('(width < 48rem)');
  const opener = document.activeElement;
  const selection = window.getSelection();
  const selectedRange =
    selection?.rangeCount && opener?.contains(selection.anchorNode)
      ? selection.getRangeAt(0).cloneRange()
      : null;
  let disposed = false;
  let pressedInside = true;

  function restoreFocus() {
    if (opener instanceof HTMLElement && opener.isConnected) {
      opener.focus({ preventScroll: true });
      if (selectedRange?.commonAncestorContainer.isConnected) {
        const current = window.getSelection();
        current?.removeAllRanges();
        current?.addRange(selectedRange);
      }
    }
  }

  function present() {
    const focused = node.contains(document.activeElement) ? document.activeElement : null;
    if (node.open) node.close();
    if (narrow.matches) node.show();
    else node.showModal();
    if (focused instanceof HTMLElement) focused.focus({ preventScroll: true });
  }

  function close() {
    if (!node.open) return;
    const restore = node.contains(document.activeElement);
    node.close();
    if (restore) restoreFocus();
    onclose();
  }

  present();
  queueMicrotask(() => {
    if (!disposed && node.open && shouldAutoFocus()) node.querySelector('input')?.focus();
  });
  narrow.addEventListener('change', present);
  const remove = [
    on(node, 'close', () => {
      // A breakpoint change or quick reopen can queue an obsolete close event.
      if (!node.open) {
        restoreFocus();
        onclose();
      }
    }),
    on(node, 'keydown', (event) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return;
      event.preventDefault();
      event.stopPropagation();
      close();
    }),
    on(node, 'cancel', (event) => {
      event.preventDefault();
      close();
    }),
    on(
      document,
      'pointerdown',
      (event) => {
        pressedInside =
          event.target instanceof Node && node.contains(event.target) && event.target !== node;
      },
      { capture: true }
    ),
    on(
      document,
      'click',
      (event) => {
        if (event.target instanceof Node && !node.contains(event.target) && narrow.matches) {
          // Close before the header's click handler, without consuming its action.
          close();
        } else if (!narrow.matches && !pressedInside && event.target === node) close();
      },
      { capture: true }
    )
  ];

  return () => {
    disposed = true;
    narrow.removeEventListener('change', present);
    remove.forEach((unsubscribe) => unsubscribe());
    if (node.open) {
      node.close();
      restoreFocus();
    }
  };
}
