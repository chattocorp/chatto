<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent, waitFor, within } from 'storybook/test';
  import MotdContent from './MotdContent.svelte';
  import MotdModal from '../../routes/chat/modals/MotdModal.svelte';

  const { Story } = defineMeta({
    title: 'UI/Message of the day content',
    component: MotdContent,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let open = $state(false);
  const motd =
    '**Maintenance window:** Saturday, 10:00–11:00 UTC. [Read details](https://example.com).\n\nPlease save your work before maintenance starts.';
</script>

<Story
  name="Markdown message"
  asChild
  play={async ({ canvas, canvasElement }) => {
    const trigger = canvas.getByRole('button', { name: 'Message of the Day' });
    const preview = canvas.getByTestId('motd-preview');
    await expect(await canvas.findByRole('link', { name: 'Read details' })).toHaveAttribute(
      'href',
      'https://example.com'
    );
    await expect(preview.querySelector('strong')).toHaveTextContent('Maintenance window:');
    await expect(preview.querySelector('br, p')).toBeNull();
    await expect(preview.scrollWidth).toBeGreaterThan(preview.clientWidth);
    await expect(getComputedStyle(preview).whiteSpace).toBe('nowrap');
    await userEvent.click(trigger);
    const body = within(canvasElement.ownerDocument.body);
    const dialog = await body.findByRole('dialog', { name: 'Message of the Day' });
    await expect(await within(dialog).findByRole('link', { name: 'Read details' })).toHaveAttribute(
      'href',
      'https://example.com'
    );
    await waitFor(() =>
      expect(
        within(dialog).getByText('Please save your work before maintenance starts.')
      ).toBeVisible()
    );
    const footer = within(dialog.querySelector('footer')!);
    await userEvent.click(footer.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(dialog).not.toBeVisible());
    await waitFor(() => expect(trigger).toHaveFocus());
  }}
>
  <div class="flex w-80 max-w-full items-center gap-3 rounded-lg bg-background p-2">
    <span class="shrink-0">Chatto</span>
    <MotdContent {motd} onclick={() => (open = true)} />
    <span class="shrink-0">v0.5</span>
  </div>
  {#if open}
    <MotdModal {motd} onclose={() => (open = false)} />
  {/if}
</Story>
