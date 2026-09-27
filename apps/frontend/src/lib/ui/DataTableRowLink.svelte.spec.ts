import '../../app.css';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from '@vitest/browser/context';
import DataTableRowLinkHarness from './DataTableRowLinkHarness.svelte';

function renderTable(linked = true) {
  const onnavigate = vi.fn();
  const oncopy = vi.fn();
  const view = render(DataTableRowLinkHarness, { props: { linked, onnavigate, oncopy } });
  const q = (testid: string) =>
    view.container.querySelector<HTMLElement>(`[data-testid="${testid}"]`)!;
  return { ...view, onnavigate, oncopy, q };
}

function click(element: Element, init: MouseEventInit = {}) {
  element.dispatchEvent(
    new MouseEvent(init.button === 1 ? 'auxclick' : 'click', {
      bubbles: true,
      cancelable: true,
      button: 0,
      ...init
    })
  );
}

afterEach(() => {
  vi.restoreAllMocks();
  window.getSelection()?.removeAllRanges();
});

describe('DataTable row links', () => {
  it('activates the link of the clicked row from a passive cell', async () => {
    const { q, onnavigate } = renderTable();

    await userEvent.click(q('note-2'));

    expect(onnavigate).toHaveBeenCalledTimes(1);
    expect(onnavigate).toHaveBeenCalledWith('2');
  });

  it('lets other controls in the row handle their own clicks', async () => {
    const { q, onnavigate, oncopy } = renderTable();

    await userEvent.click(q('copy-3'));

    expect(oncopy).toHaveBeenCalledWith('3');
    expect(onnavigate).not.toHaveBeenCalled();
  });

  it('respects Svelte handlers inside the row that cancel the click', async () => {
    const { q, onnavigate } = renderTable();

    await userEvent.click(q('handled-1'));

    expect(onnavigate).not.toHaveBeenCalled();
  });

  it('prevents middle-click autoscroll on a linked row', () => {
    const { q } = renderTable();
    const press = new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 1 });

    q('note-1').dispatchEvent(press);

    expect(press.defaultPrevented).toBe(true);
  });

  it('opens the row link in a new tab for modified and middle clicks', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const { q, onnavigate } = renderTable();

    click(q('note-1'), { ctrlKey: true });
    click(q('note-2'), { button: 1 });

    expect(open).toHaveBeenNthCalledWith(
      1,
      expect.stringMatching(/#member-1$/),
      '_blank',
      'noopener'
    );
    expect(open).toHaveBeenNthCalledWith(
      2,
      expect.stringMatching(/#member-2$/),
      '_blank',
      'noopener'
    );
    expect(onnavigate).not.toHaveBeenCalled();
  });

  it('keeps a text selection instead of navigating', () => {
    const { q, onnavigate } = renderTable();
    const note = q('note-1');
    const range = document.createRange();
    range.selectNodeContents(note);
    window.getSelection()!.addRange(range);

    click(note);

    expect(onnavigate).not.toHaveBeenCalled();
  });

  it('ignores a selection outside the clicked row', async () => {
    const { q, onnavigate } = renderTable();
    const range = document.createRange();
    range.selectNodeContents(q('note-1'));
    window.getSelection()!.addRange(range);

    click(q('note-2'));

    expect(onnavigate).toHaveBeenCalledWith('2');
  });

  it('reaches the row link with the keyboard and highlights its row', async () => {
    const { container } = renderTable();
    const link = container.querySelector<HTMLAnchorElement>('.data-table-row-link')!;
    const row = link.closest('tr')!;
    const restingBackground = getComputedStyle(row).backgroundColor;

    await userEvent.tab();

    expect(document.activeElement).toBe(link);
    expect(getComputedStyle(row).backgroundColor).not.toBe(restingBackground);
  });

  it('marks only linked rows as pointer targets', () => {
    const linked = renderTable(true);
    expect(getComputedStyle(linked.q('note-1').closest('tr')!).cursor).toBe('pointer');
    linked.unmount();

    const passive = renderTable(false);
    const note = passive.q('note-1');
    expect(getComputedStyle(note.closest('tr')!).cursor).not.toBe('pointer');
    click(note);
    expect(passive.onnavigate).not.toHaveBeenCalled();
  });
});
