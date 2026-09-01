import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import ConfirmDialog from './ConfirmDialog.svelte';

describe('ConfirmDialog', () => {
  it('uses the caller-provided loading label and locks both actions', async () => {
    const { getByRole } = render(ConfirmDialog, {
      props: {
        visible: true,
        title: 'Delete Role',
        actionLabel: 'Delete Role',
        actionLoadingLabel: 'Deleting Role…',
        loading: true,
        children: testSnippet('<p>This action cannot be undone.</p>'),
        onconfirm: vi.fn(),
        onclose: vi.fn()
      }
    });

    const cancel = getByRole('button', { name: 'Cancel' });
    const confirm = getByRole('button', { name: 'Deleting Role…' });
    await expect.element(cancel).toBeDisabled();
    await expect.element(confirm).toBeDisabled();
  });
});
