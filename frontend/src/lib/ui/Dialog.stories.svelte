<script module>
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Dialog from './Dialog.svelte';

  const { Story } = defineMeta({
    title: 'UI/Dialog',
    component: Dialog,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let dialogVisible = $state(false);
  let dialogWithoutTitleVisible = $state(false);
  let smallDialogVisible = $state(false);
  let largeDialogVisible = $state(false);
  let dialogWithFooterVisible = $state(false);
</script>

<Story name="Default (with title)" asChild>
  <button
    class="rounded bg-primary px-4 py-2 text-white hover:bg-primary-hover"
    onclick={() => (dialogVisible = true)}
  >
    Open Dialog
  </button>

  <Dialog bind:visible={dialogVisible} title="Dialog Title">
    <p>This is the dialog content. It can contain any elements you want.</p>
    <p class="mt-2">
      Click outside the dialog to dismiss it. The dialog uses a blurred background overlay.
    </p>
  </Dialog>
</Story>

<Story name="Without Title" asChild>
  <button
    class="rounded bg-primary px-4 py-2 text-white hover:bg-primary-hover"
    onclick={() => (dialogWithoutTitleVisible = true)}
  >
    Open Dialog Without Title
  </button>

  <Dialog bind:visible={dialogWithoutTitleVisible}>
    <p>This dialog has no title, just content.</p>
    <p class="mt-2">The header section is completely omitted when no title is provided.</p>
  </Dialog>
</Story>

<Story name="Small Size" asChild>
  <button
    class="rounded bg-primary px-4 py-2 text-white hover:bg-primary-hover"
    onclick={() => (smallDialogVisible = true)}
  >
    Open Small Dialog
  </button>

  <Dialog bind:visible={smallDialogVisible} title="Small Dialog" size="sm">
    <p>This is a small dialog (w-100 max-w-[60vw]).</p>
    <p class="mt-2">Perfect for simple confirmations or short messages.</p>
  </Dialog>
</Story>

<Story name="Large Size" asChild>
  <button
    class="rounded bg-primary px-4 py-2 text-white hover:bg-primary-hover"
    onclick={() => (largeDialogVisible = true)}
  >
    Open Large Dialog
  </button>

  <Dialog bind:visible={largeDialogVisible} title="Large Dialog" size="lg">
    <p>This is a large dialog (w-200 max-w-[90vw]).</p>
    <p class="mt-2">Useful for more complex forms or detailed content.</p>
  </Dialog>
</Story>

<Story name="With Footer" asChild>
  <button
    class="rounded bg-primary px-4 py-2 text-white hover:bg-primary-hover"
    onclick={() => (dialogWithFooterVisible = true)}
  >
    Open Dialog With Footer
  </button>

  <Dialog bind:visible={dialogWithFooterVisible} title="Confirm Action">
    <p>Are you sure you want to perform this action?</p>

    {#snippet footer()}
      <div class="flex justify-end gap-2">
        <button
          class="rounded border border-border bg-surface px-4 py-2 text-text hover:bg-surface-highlighted"
          onclick={() => (dialogWithFooterVisible = false)}
        >
          Cancel
        </button>
        <button
          class="rounded bg-primary px-4 py-2 text-white hover:bg-primary-hover"
          onclick={() => (dialogWithFooterVisible = false)}
        >
          Confirm
        </button>
      </div>
    {/snippet}
  </Dialog>
</Story>
