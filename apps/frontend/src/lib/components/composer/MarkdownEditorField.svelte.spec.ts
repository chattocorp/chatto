import '../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import type { RoomMember } from '$lib/state/room';
import MarkdownEditorField from './MarkdownEditorField.svelte';

/** Controlled directory responses exercise editor search ordering and cleanup. */
function directory() {
  const requests = new Map<string, (members: RoomMember[]) => void>();
  const searchMembers = vi.fn(
    (query: string) => new Promise<RoomMember[]>((resolve) => requests.set(query, resolve))
  );
  return { requests, searchMembers };
}

const bob: RoomMember = {
  id: 'bob',
  login: 'bob',
  displayName: 'Bob Example',
  avatarUrl: null,
  presenceStatus: 0
};

describe('MarkdownEditorField mention lifecycle', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it.each(['markdown', 'visual'] as const)(
    'closes the picker and discards pending search when the %s draft is replaced',
    async (editorKind) => {
      const { requests, searchMembers } = directory();
      const view = render(MarkdownEditorField, {
        editorKind,
        label: 'Notice',
        placeholder: 'Write a notice',
        mentions: { members: [bob], searchMembers }
      });
      const editor = page.getByRole('textbox', { name: 'Write a notice', exact: true });
      await editor.fill('@bo');
      await expect.poll(() => searchMembers.mock.calls.length).toBe(1);
      await expect.element(page.getByTestId('mention-autocomplete')).toBeVisible();
      await view.rerender({ value: 'Replacement draft' });
      requests.get('bo')!([bob]);
      await expect.element(editor).toHaveTextContent('Replacement draft');
      await expect.element(page.getByTestId('mention-autocomplete')).not.toBeInTheDocument();
      await userEvent.keyboard('{Enter}');
      await expect.element(editor).not.toHaveTextContent('@bob');
    }
  );

  it('ignores an older search response and keeps the picker closed after Escape', async () => {
    const { requests, searchMembers } = directory();
    render(MarkdownEditorField, {
      editorKind: 'markdown',
      label: 'Notice',
      placeholder: 'Write a notice',
      mentions: { members: [], searchMembers }
    });
    const editor = page.getByRole('textbox', { name: 'Write a notice', exact: true });
    await editor.fill('@b');
    await expect.poll(() => searchMembers.mock.calls.length).toBe(1);
    await editor.fill('@bo');
    await expect.poll(() => searchMembers.mock.calls.length).toBe(2);
    requests.get('bo')!([bob]);
    await expect.element(page.getByTestId('mention-autocomplete')).toHaveTextContent('Bob Example');
    requests.get('b')!([{ ...bob, id: 'old', displayName: 'Old result' }]);
    await expect
      .element(page.getByTestId('mention-autocomplete'))
      .not.toHaveTextContent('Old result');
    await userEvent.keyboard('{Escape}');
    await expect.element(page.getByTestId('mention-autocomplete')).not.toBeInTheDocument();
  });
});
