import '../../app.css';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from '@vitest/browser/context';
import DataTableRowLinkHarness from './DataTableRowLinkHarness.svelte';

function renderLinkedTable(linked = true) {
  return render(DataTableRowLinkHarness, { props: { linked } });
}

function centreOf(element: Element) {
  const rect = element.getBoundingClientRect();
  return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
}

describe('DataTable row links', () => {
  it('makes the whole row a pointer target for its row link', () => {
    const { container } = renderLinkedTable();
    const row = container.querySelector('tbody tr')!;
    const note = container.querySelector('[data-testid="note"]')!;
    const { x, y } = centreOf(note);

    expect(getComputedStyle(row).cursor).toBe('pointer');
    expect(document.elementFromPoint(x, y)).toBe(container.querySelector('.data-table-row-link'));
  });

  it('keeps other controls in the row above the row link', () => {
    const { container } = renderLinkedTable();
    const copy = container.querySelector('[data-testid="copy"]')!;
    const { x, y } = centreOf(copy);

    expect(document.elementFromPoint(x, y)).toBe(copy);
  });

  it('reaches the row link with the keyboard', async () => {
    const { container } = renderLinkedTable();
    const link = container.querySelector<HTMLAnchorElement>('.data-table-row-link')!;

    await userEvent.tab();

    expect(document.activeElement).toBe(link);
    expect(link.getAttribute('href')).toBe('#member-1');
  });

  it('leaves rows without a row link passive', () => {
    const { container } = renderLinkedTable(false);
    const row = container.querySelector('tbody tr')!;

    expect(getComputedStyle(row).cursor).not.toBe('pointer');
  });
});
