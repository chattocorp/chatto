import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import DataTable from './DataTable.svelte';

function renderTable(props: { hoverable?: boolean } = {}) {
  return render(DataTable, {
    props: {
      items: [{ id: '1', name: 'Alice' }],
      columns: 1,
      header: testSnippet('<th>Name</th>'),
      // The row snippet receives the item as `params`. We inline a
      // self-contained row that ignores it — `testSnippet` builds a
      // generic Snippet from the HTML string.
      row: testSnippet('<td data-testid="row-cell">cell</td>'),
      ...props
    }
  });
}

describe('DataTable.hoverable', () => {
  it('applies hover bg by default', async () => {
    const { container } = renderTable();
    const tr = container.querySelector('tbody tr') as HTMLElement;
    expect(tr.className).toContain('hover:bg-surface-200/40');
  });

  it('omits hover bg when hoverable=false', async () => {
    const { container } = renderTable({ hoverable: false });
    const tr = container.querySelector('tbody tr') as HTMLElement;
    expect(tr.className).not.toContain('hover:bg-surface-200/40');
  });

  it('still renders cursor-pointer on hoverable=false rows when onRowClick is set', async () => {
    const onRowClick = vi.fn();
    const { container } = render(DataTable, {
      props: {
        items: [{ id: '1' }],
        columns: 1,
        header: testSnippet('<th>X</th>'),
        row: testSnippet('<td>x</td>'),
        hoverable: false,
        onRowClick
      }
    });
    const tr = container.querySelector('tbody tr') as HTMLElement;
    expect(tr.className).toContain('cursor-pointer');
  });
});
