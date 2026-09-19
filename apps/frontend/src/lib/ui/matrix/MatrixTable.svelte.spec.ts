import '../../../app.css';
import { beforeEach, describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import MatrixTableTestHarness from './MatrixTableTestHarness.svelte';

describe('MatrixTable', () => {
  beforeEach(async () => { await page.viewport(1280, 900); });
  it.each(['ltr', 'rtl'])('stacks labels in narrow containers and scrolls aligned cells in %s', async (direction) => {
    const host = document.createElement('div');
    host.style.width = '320px';
    host.dir = direction;
    document.body.append(host);
    try {
      const { container } = render(MatrixTableTestHarness, {
        target: host,
        props: {
          rows: [{ id: 'long', label: 'A long permission label that must wrap in a narrow container' }],
          columns: Array.from({ length: 16 }, (_, index) => ({
            id: `column-${index}`, label: `Role ${index}`, kind: 'room' as const
          }))
        }
      });
      await expect.poll(() => container.querySelectorAll('tbody tr').length).toBe(2);
      const heading = container.querySelector('[data-test-row-heading="long"]')!;
      const cell = container.querySelector('td[data-test-cell]')!;
      const column = container.querySelector('th[data-test-column]')!;
      const viewport = container.querySelector('.data-table-viewport')!;
      expect(container.querySelectorAll('.data-table-viewport')).toHaveLength(1);
      expect(heading.getBoundingClientRect().bottom).toBeLessThanOrEqual(cell.getBoundingClientRect().top);
      expect(heading.parentElement!.getBoundingClientRect().width).toBe(320);
      expect(cell.getAttribute('headers')?.split(' ')).toEqual([heading.closest('th')!.id, column.id]);
      const labelStart = heading.getBoundingClientRect().left;
      const cellStart = cell.getBoundingClientRect().left;
      viewport.scrollLeft = direction === 'ltr' ? 120 : -120;
      await expect.poll(() => Math.abs(viewport.scrollLeft)).toBe(120);
      expect(heading.getBoundingClientRect().left).toBeCloseTo(labelStart);
      expect(cell.getBoundingClientRect().left).not.toBe(cellStart);
      expect(cell.getBoundingClientRect().left).toBeCloseTo(column.getBoundingClientRect().left);

      host.style.width = '800px';
      await expect.poll(() => container.querySelectorAll('tbody tr').length).toBe(1);
      expect(container.querySelector('tbody th')?.parentElement).toBe(cell.parentElement);
    } finally {
      host.remove();
    }
  });

  it('renders domain-neutral rows, columns, attributes, and rotated headings', () => {
    const { container } = render(MatrixTableTestHarness);

    expect(container.querySelector('thead')?.textContent).toContain('Activity');
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
    expect(container.querySelectorAll('th[data-test-column]')).toHaveLength(2);
    expect(container.querySelectorAll('td[data-test-cell]')).toHaveLength(4);

    const heading = container.querySelector('[data-test-column-heading="server"]')!;
    const headingShell = heading.parentElement!;
    expect(headingShell.className).toContain('[writing-mode:vertical-rl]');
    expect(headingShell.className).toContain('rotate-180');

    const rowHeading = container
      .querySelector('[data-test-row-heading="mentions"]')!
      .closest('th')!;
    expect(rowHeading.className).toContain('text-start');
    expect(rowHeading.className).toContain('font-normal');
  });

  it('coordinates hover highlighting across the row, column, and intersection', () => {
    const { container } = render(MatrixTableTestHarness);
    const intersection = container.querySelector(
      '[data-test-cell="mentions:general"]'
    ) as HTMLTableCellElement;
    const sameRow = container.querySelector(
      '[data-test-cell="mentions:server"]'
    ) as HTMLTableCellElement;
    const sameColumn = container.querySelector(
      '[data-test-cell="replies:general"]'
    ) as HTMLTableCellElement;
    const unrelated = container.querySelector(
      '[data-test-cell="replies:server"]'
    ) as HTMLTableCellElement;

    intersection.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    expect(intersection.className).toContain('bg-action/15');
    expect(sameRow.className).toContain('bg-action/8');
    expect(sameColumn.className).toContain('bg-action/8');
    expect(unrelated.className).toContain('bg-surface-emphasized/40');
    expect(
      container
        .querySelector('[data-test-row-heading="mentions"]')
        ?.getAttribute('data-highlighted')
    ).toBe('true');
    expect(
      container
        .querySelector('[data-test-column-heading="general"]')
        ?.getAttribute('data-highlighted')
    ).toBe('true');
    expect(container.querySelector('[data-test-column-heading="general"]')?.className).toContain(
      'text-action'
    );

    intersection.dispatchEvent(new MouseEvent('mouseleave'));
    flushSync();
    expect(intersection.className).toContain('bg-surface-emphasized/20');
  });

  it('retains hover during an inert check but clears it when the pointer moves away', () => {
    const { container } = render(MatrixTableTestHarness);
    const cell = container.querySelector<HTMLElement>('[data-test-cell="mentions:general"]')!;
    cell.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    const bounds = cell.getBoundingClientRect();
    container.inert = true;
    cell.dispatchEvent(new MouseEvent('mouseleave', {
      clientX: bounds.left + bounds.width / 2, clientY: bounds.top + bounds.height / 2
    }));
    flushSync();
    expect(cell.className).toContain('bg-action/15');
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: bounds.right + 10, clientY: bounds.bottom + 10 }));
    flushSync();
    expect(cell.className).not.toContain('bg-action/');
    container.inert = false;
  });

  it('retains keyboard highlighting through inert blur until focus moves elsewhere', () => {
    const { container } = render(MatrixTableTestHarness);
    const cell = container.querySelector<HTMLElement>('[data-test-cell="replies:server"]')!;
    cell.querySelector('button')!.focus();
    flushSync();
    container.inert = true;
    cell.dispatchEvent(new FocusEvent('focusout', { bubbles: true }));
    flushSync();
    expect(cell.className).toContain('bg-action/15');
    container.inert = false;
    container.querySelector<HTMLButtonElement>('[data-test-cell="mentions:general"] button')!.focus();
    flushSync();
    expect(cell.className).not.toContain('bg-action/15');
  });

  it('retains coordinated highlighting while a cell control has keyboard focus', () => {
    const { container } = render(MatrixTableTestHarness);
    const cell = container.querySelector(
      '[data-test-cell="replies:server"]'
    ) as HTMLTableCellElement;
    const button = cell.querySelector('button')!;

    button.focus();
    flushSync();

    expect(cell.className).toContain('bg-action/15');
    expect(
      container.querySelector('[data-test-row-heading="replies"]')?.getAttribute('data-highlighted')
    ).toBe('true');
    expect(
      container
        .querySelector('[data-test-column-heading="server"]')
        ?.getAttribute('data-highlighted')
    ).toBe('true');

    button.blur();
    flushSync();
    expect(cell.className).toContain('bg-surface-emphasized/40');
  });

  it('does not retain coordinated highlighting after pointer activation', async () => {
    const { container } = render(MatrixTableTestHarness);
    const cell = container.querySelector(
      '[data-test-cell="mentions:general"]'
    ) as HTMLTableCellElement;
    const button = cell.querySelector('button')!;

    cell.dispatchEvent(new MouseEvent('mouseenter'));
    await userEvent.click(button);
    expect(document.activeElement).toBe(button);

    cell.dispatchEvent(new MouseEvent('mouseleave'));
    flushSync();

    expect(cell.className).toContain('bg-surface-emphasized/20');
    expect(cell.className).not.toContain('bg-action/');
    expect(
      container
        .querySelector('[data-test-row-heading="mentions"]')
        ?.getAttribute('data-highlighted')
    ).toBe('false');
    expect(
      container
        .querySelector('[data-test-column-heading="general"]')
        ?.getAttribute('data-highlighted')
    ).toBe('false');
    expect(container.querySelector('[data-test-column-heading="general"]')?.className).toContain(
      'text-muted'
    );
  });

  it('does not highlight cells that the adapter marks as non-interactive', () => {
    const { container } = render(MatrixTableTestHarness, {
      props: { interactive: false }
    });
    const cell = container.querySelector(
      '[data-test-cell="mentions:general"]'
    ) as HTMLTableCellElement;

    cell.dispatchEvent(new MouseEvent('mouseenter'));
    cell.querySelector('button')?.focus();
    flushSync();

    expect(cell.className).not.toContain('bg-action/');
    expect(
      container
        .querySelector('[data-test-row-heading="mentions"]')
        ?.getAttribute('data-highlighted')
    ).toBe('false');
  });

  it('shows the consumer-provided empty state', () => {
    const { container } = render(MatrixTableTestHarness, { props: { rows: [] } });

    expect(container.querySelector('tbody')?.textContent).toContain('No matrix rows');
  });

  it('preserves keyed row and cell elements when consumers reorder their data', async () => {
    const rows = [
      { id: 'mentions', label: 'Mentions' },
      { id: 'replies', label: 'Replies' }
    ];
    const columns = [
      { id: 'server', label: 'Example server', kind: 'server' as const },
      { id: 'general', label: 'General', kind: 'group' as const }
    ];
    const rendered = render(MatrixTableTestHarness, { props: { rows, columns } });
    const originalCell = rendered.container.querySelector('[data-test-cell="mentions:server"]');
    const originalHeading = rendered.container.querySelector('[data-test-column="server"]');

    await rendered.rerender({ rows: [...rows].reverse(), columns: [...columns].reverse() });

    expect(rendered.container.querySelector('[data-test-cell="mentions:server"]')).toBe(
      originalCell
    );
    expect(rendered.container.querySelector('[data-test-column="server"]')).toBe(originalHeading);
  });
});
