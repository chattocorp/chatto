<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent, waitFor, within } from 'storybook/test';
  import { Button } from '$lib/ui/form';
  import type { ShortcutDescription } from '$lib/keyboardShortcuts';
  import KeyboardShortcutHelp from './KeyboardShortcutHelp.svelte';

  const shortcuts: ShortcutDescription[] = [
    { group: 'app', label: 'Open quick switcher', key: 'k', command: true },
    { group: 'app', label: 'Keyboard shortcuts', key: 'k', command: true, shift: true },
    { group: 'app', label: 'Go to next unread source', key: 'ArrowDown', alt: true, shift: true },
    { group: 'editor', label: 'Send message', key: 'Enter', command: true },
    { group: 'editor', label: 'New line', key: 'Enter' },
    { group: 'editor', label: 'Bold', key: 'b', command: true },
    { group: 'editor', label: 'Italic', key: 'i', command: true },
    { group: 'editor', label: 'Underline', key: 'u', command: true },
    { group: 'editor', label: 'Undo', key: 'z', command: true },
    { group: 'editor', label: 'Redo', key: 'z', command: true, shift: true },
    { group: 'editor', label: 'Complete mention or emoji', key: 'Tab' },
    { group: 'editor', label: 'Edit last message (empty draft)', key: 'ArrowUp' }
  ];

  const { Story } = defineMeta({
    title: 'Navigation/Keyboard shortcut help',
    component: KeyboardShortcutHelp,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let visible = $state(false);
  let appOnlyVisible = $state(false);
</script>

<Story
  name="App and editor shortcuts"
  asChild
  play={async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    const trigger = canvas.getByRole('button', { name: 'Open shortcut help' });
    await userEvent.click(trigger);
    const dialog = await page.findByRole('dialog', { name: 'Keyboard shortcuts' });
    await waitFor(() => expect(dialog).toBeVisible());
    const help = within(dialog);
    await expect(help.getByRole('group', { name: 'App' })).toBeVisible();
    await expect(help.getByRole('group', { name: 'Composer' })).toBeVisible();
    const search = help.getByRole('textbox', { name: 'Search' });
    await userEvent.type(search, 'Bold');
    await expect(help.getByText('Bold', { exact: true })).toBeVisible();
    await expect(help.queryByText('Undo', { exact: true })).not.toBeInTheDocument();
    await userEvent.clear(search);
    await userEvent.type(search, 'no matching shortcut');
    await expect(help.getByRole('status')).toBeVisible();
    await userEvent.click(help.getByRole('button', { name: 'Esc' }));
    await waitFor(() => expect(search).not.toBeInTheDocument());
    await expect(trigger).toHaveFocus();
    await userEvent.click(trigger);
    const reopened = await page.findByRole('dialog', { name: 'Keyboard shortcuts' });
    await expect(within(reopened).getByRole('textbox', { name: 'Search' })).toHaveValue('');
  }}
>
  <Button onclick={() => (visible = true)}>Open shortcut help</Button>
  <KeyboardShortcutHelp bind:visible {shortcuts} />
</Story>

<Story name="App shortcuts without an editor" asChild>
  <Button onclick={() => (appOnlyVisible = true)}>Open app shortcuts</Button>
  <KeyboardShortcutHelp
    bind:visible={appOnlyVisible}
    shortcuts={shortcuts.filter((entry) => entry.group === 'app')}
  />
</Story>
