import type { Attachment } from 'svelte/attachments';

/** Class that marks the real link which a DataTable row navigates to. */
export const ROW_LINK_CLASS = 'data-table-row-link';

/**
 * Elements that handle their own pointer input. A click on one of them, or
 * inside one, never activates the row link.
 */
const INTERACTIVE_SELECTOR =
  'a, button, input, select, textarea, label, summary, [role="button"], [role="link"], [tabindex]';

/**
 * Forwards pointer clicks on DataTable body rows to each row's
 * `data-table-row-link`.
 *
 * The link stays the only keyboard stop and the accessible name of the row;
 * this attachment only adds the larger pointer target. A plain primary click
 * activates the link, so client-side routing applies. A modified or middle
 * click opens the link in a new tab, like a click on the link itself. Clicks
 * on other interactive elements, clicks that end a text selection, and clicks
 * that another handler already cancelled are ignored.
 *
 * The listener uses delegation on the table body instead of an overlay on
 * the link, because Safari does not treat a positioned table row as the
 * containing block of an absolutely positioned overlay.
 */
export const forwardRowClicks: Attachment<HTMLElement> = (body) => {
  function handle(event: MouseEvent) {
    if (event.defaultPrevented) return;
    if (event.type === 'click' && event.button !== 0) return;
    if (event.type === 'auxclick' && event.button !== 1) return;

    const target = event.target;
    if (!(target instanceof Element)) return;

    const row = target.closest('tr');
    if (!row || !body.contains(row)) return;
    // Only look inside the row: a scroll container around the table can be
    // focusable, and it must not disable row clicks.
    const interactive = target.closest(INTERACTIVE_SELECTOR);
    if (interactive && row.contains(interactive)) return;
    const link = row.querySelector<HTMLAnchorElement>(`a.${ROW_LINK_CLASS}[href]`);
    if (!link) return;

    // Ending a drag selection inside a row fires a click; keep the selection.
    if (window.getSelection()?.toString()) return;

    const newTab = event.type === 'auxclick' || event.metaKey || event.ctrlKey || event.shiftKey;
    if (newTab) {
      window.open(link.href, '_blank', 'noopener');
    } else {
      link.click();
    }
  }

  body.addEventListener('click', handle);
  body.addEventListener('auxclick', handle);
  return () => {
    body.removeEventListener('click', handle);
    body.removeEventListener('auxclick', handle);
  };
};
