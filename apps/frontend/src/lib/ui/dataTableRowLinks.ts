import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';

/** Class that marks the real link which a DataTable row navigates to. */
const ROW_LINK_CLASS = 'data-table-row-link';

/**
 * Elements that handle their own pointer input. A click on one of them, or
 * inside one, never activates the row link.
 */
const INTERACTIVE_SELECTOR =
  'a, button, input, select, textarea, label, summary, [role="button"], [role="link"], [tabindex]';

/**
 * Returns the row link that a pointer event on a passive part of a row should
 * activate, or null when the event belongs to something else.
 */
function rowLinkFor(body: HTMLElement, event: MouseEvent): HTMLAnchorElement | null {
  if (event.defaultPrevented) return null;

  const target = event.target;
  if (!(target instanceof Element)) return null;

  const row = target.closest('tr');
  if (!row || !body.contains(row)) return null;
  // Only look inside the row: a scroll container around the table can be
  // focusable, and it must not disable row clicks.
  const interactive = target.closest(INTERACTIVE_SELECTOR);
  if (interactive && row.contains(interactive)) return null;

  return row.querySelector<HTMLAnchorElement>(`a.${ROW_LINK_CLASS}[href]`);
}

/**
 * Forwards pointer clicks on DataTable body rows to each row's
 * `data-table-row-link`.
 *
 * The link stays the only keyboard stop and the accessible name of the row;
 * this attachment only adds the larger pointer target. A plain primary click
 * activates the link, so client-side routing applies. A modified or middle
 * click opens the link in a new tab. Clicks on other interactive elements,
 * clicks that end a text selection, and clicks that a handler inside the row
 * cancelled or stopped are ignored. The listeners use `svelte/events`, so
 * Svelte event handlers inside the row run first.
 *
 * The listener uses delegation on the table body instead of an overlay on
 * the link, because Safari does not treat a positioned table row as the
 * containing block of an absolutely positioned overlay.
 */
export const forwardRowClicks: Attachment<HTMLElement> = (body) => {
  function activate(event: MouseEvent) {
    if (event.type === 'click' && event.button !== 0) return;
    if (event.type === 'auxclick' && event.button !== 1) return;

    const link = rowLinkFor(body, event);
    if (!link) return;

    // Ending a drag selection inside the row fires a click; keep the selection.
    const selection = window.getSelection();
    const row = link.closest('tr');
    if (selection && !selection.isCollapsed && row && selection.containsNode(row, true)) return;

    if (event.type === 'auxclick' || event.metaKey || event.ctrlKey || event.shiftKey) {
      window.open(link.href, '_blank', 'noopener');
    } else {
      link.click();
    }
  }

  // A middle press on a linked row opens a tab, so it must not also start the
  // browser's autoscroll mode.
  function preventAutoscroll(event: MouseEvent) {
    if (event.button === 1 && rowLinkFor(body, event)) event.preventDefault();
  }

  const listeners = [
    on(body, 'click', activate),
    on(body, 'auxclick', activate),
    on(body, 'mousedown', preventAutoscroll)
  ];
  return () => listeners.forEach((off) => off());
};
