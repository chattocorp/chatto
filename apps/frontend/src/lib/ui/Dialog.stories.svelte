<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import Dialog from './Dialog.svelte';

  const componentDescription = `
    Use Dialog for focused overlays that need custom body content. Use FormDialog for submit/cancel
    forms and ConfirmDialog for destructive or high-risk confirmations.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Dialog',
    component: Dialog,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  let dialogVisible = $state(false);
  let dialogWithoutTitleVisible = $state(false);
  let smallDialogVisible = $state(false);
  let largeDialogVisible = $state(false);
  let dialogWithFooterVisible = $state(false);
</script>

<Story
  name="Default (with title)"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Default dialog with title, close affordance, overlay, and slotted body content.'
      }
    }
  }}
>
  <Button onclick={() => (dialogVisible = true)}>Open Dialog</Button>

  <Dialog bind:visible={dialogVisible} title="Dialog Title">
    <p>This is the dialog content. It can contain any elements you want.</p>
    <p class="mt-2">
      Click outside the dialog to dismiss it. The dialog uses a blurred background overlay.
    </p>
  </Dialog>
</Story>

<Story
  name="Without Title"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Titleless dialogs are for compact content where the trigger already names the task.'
      }
    }
  }}
>
  <Button onclick={() => (dialogWithoutTitleVisible = true)}>Open Dialog Without Title</Button>

  <Dialog bind:visible={dialogWithoutTitleVisible}>
    <p>This dialog has no title, just content.</p>
    <p class="mt-2">The header section is completely omitted when no title is provided.</p>
  </Dialog>
</Story>

<Story
  name="Small Size"
  asChild
  parameters={{
    docs: {
      description: { story: 'Small dialogs fit short messages and simple confirmation context.' }
    }
  }}
>
  <Button onclick={() => (smallDialogVisible = true)}>Open Small Dialog</Button>

  <Dialog bind:visible={smallDialogVisible} title="Small Dialog" size="sm">
    <p>This is a small dialog (w-100 max-w-[60vw]).</p>
    <p class="mt-2">Perfect for simple confirmations or short messages.</p>
  </Dialog>
</Story>

<Story
  name="Large Size"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Large dialogs provide room for denser custom content without creating nested cards.'
      }
    }
  }}
>
  <Button onclick={() => (largeDialogVisible = true)}>Open Large Dialog</Button>

  <Dialog bind:visible={largeDialogVisible} title="Large Dialog" size="lg">
    <p>This is a large dialog (w-200 max-w-[90vw]).</p>
    <p class="mt-2">Useful for more complex forms or detailed content.</p>
  </Dialog>
</Story>

<Story
  name="With Footer"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Footer actions follow the shared modal pattern: horizontal, right-aligned, secondary cancel, primary action.'
      }
    }
  }}
>
  <Button onclick={() => (dialogWithFooterVisible = true)}>Open Dialog With Footer</Button>

  <Dialog bind:visible={dialogWithFooterVisible} title="Confirm Action">
    <p>Are you sure you want to perform this action?</p>

    {#snippet footer()}
      <div class="flex flex-wrap justify-end gap-2">
        <Button variant="secondary" onclick={() => (dialogWithFooterVisible = false)}>
          Cancel
        </Button>
        <Button onclick={() => (dialogWithFooterVisible = false)}>Confirm</Button>
      </div>
    {/snippet}
  </Dialog>
</Story>
