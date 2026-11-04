<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent, waitFor } from 'storybook/test';
  import PreviewActionSheet from './PreviewActionSheet.svelte';
  import PreviewAction from './PreviewAction.svelte';

  const { Story } = defineMeta({
    title: 'UI/Preview action sheet',
    component: PreviewActionSheet,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let paused = $state(false);
  let open = $state(false);
</script>

<Story
  name="Action sheet"
  asChild
  play={async ({ canvas }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Open action sheet' }));
    const pause = await canvas.findByRole('button', { name: 'Pause animation' });
    await expect(canvas.getByRole('button', { name: 'Unavailable action' })).toBeDisabled();
    await userEvent.click(pause);
    await waitFor(() => expect(canvas.queryByRole('dialog')).toBeNull());
    await userEvent.click(canvas.getByRole('button', { name: 'Open action sheet' }));
    await expect(canvas.getByRole('button', { name: 'Pause animation' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
  }}
>
  <button type="button" class="btn-secondary" onclick={() => (open = true)}
    >Open action sheet</button
  >
  {#if open}<PreviewActionSheet onclose={() => (open = false)}>
      <PreviewAction
        label="Pause animation"
        icon="icon-[uil--pause]"
        aria-pressed={paused}
        onclick={() => (paused = !paused)}
      />
      <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
      <PreviewAction label="Unavailable action" icon="icon-[uil--trash-alt]" disabled />
    </PreviewActionSheet>{/if}
</Story>
