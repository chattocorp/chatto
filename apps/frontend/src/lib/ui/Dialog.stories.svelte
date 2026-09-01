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
  let referenceDialogVisible = $state(false);
  let longDialogVisible = $state(false);
  let narrowDialogVisible = $state(false);
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
  name="Reference multi-action dialog"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The standard framed tray, inset work plane, semantic actions, and action-specific icons match the canonical modal direction.'
      }
    }
  }}
>
  <Button onclick={() => (referenceDialogVisible = true)}>Open sign-out dialog</Button>

  <Dialog bind:visible={referenceDialogVisible} title="Sign Out" size="md">
    <p class="text-muted">
      Sign out of only the selected server, or disconnect every server from this client.
    </p>

    {#snippet footer()}
      <Button variant="secondary" onclick={() => (referenceDialogVisible = false)}>Cancel</Button>
      <Button defaultAction onclick={() => (referenceDialogVisible = false)}>
        <span class="iconify icon-[uil--sign-out-alt]"></span>
        Current Server
      </Button>
      <Button variant="danger" onclick={() => (referenceDialogVisible = false)}>
        <span class="iconify icon-[uil--signout]"></span>
        All Servers
      </Button>
    {/snippet}
  </Dialog>
</Story>

<Story
  name="Long scrolling content"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'The title and actions remain visible while only the dialog body scrolls.'
      }
    }
  }}
>
  <Button onclick={() => (longDialogVisible = true)}>Open long dialog</Button>

  <Dialog bind:visible={longDialogVisible} title="Review Changes" size="md">
    <div class="flex flex-col gap-4 text-muted">
      {#each Array(14) as _, index (index)}
        <p>
          Change {index + 1}: Review this item before you apply the configuration to the server.
        </p>
      {/each}
    </div>

    {#snippet footer()}
      <Button variant="secondary" onclick={() => (longDialogVisible = false)}>Cancel</Button>
      <Button defaultAction onclick={() => (longDialogVisible = false)}>
        <span class="iconify icon-[uil--check]"></span>
        Apply Changes
      </Button>
    {/snippet}
  </Dialog>
</Story>

<Story
  name="Narrow action truncation"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Resize the canvas to a narrow viewport. Actions stay in one row and long labels truncate before the row can wrap.'
      }
    }
  }}
>
  <Button onclick={() => (narrowDialogVisible = true)}>Open narrow dialog</Button>

  <Dialog bind:visible={narrowDialogVisible} title="Choose Destination" size="md">
    <p class="text-muted">Choose where Chatto should post this message.</p>

    {#snippet footer()}
      <Button variant="secondary" onclick={() => (narrowDialogVisible = false)}>Cancel</Button>
      <Button variant="secondary" onclick={() => (narrowDialogVisible = false)}>
        Post as a Completely New Message
      </Button>
      <Button defaultAction onclick={() => (narrowDialogVisible = false)}>
        <span class="iconify icon-[uil--comment-alt-lines]"></span>
        Continue in the Existing Thread
      </Button>
    {/snippet}
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
    <p>This is a small dialog with a 400-pixel target width and standard viewport gutters.</p>
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
    <p>This is a large dialog with an 800-pixel target width and standard viewport gutters.</p>
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
          'Footer actions follow the shared modal pattern: horizontal, right-aligned, secondary cancel, recommended action.'
      }
    }
  }}
>
  <Button onclick={() => (dialogWithFooterVisible = true)}>Open Dialog With Footer</Button>

  <Dialog bind:visible={dialogWithFooterVisible} title="Continue your previous thread?" size="md">
    <p class="text-muted">
      Your previous message already has a thread. Where should this message go?
    </p>

    {#snippet footer()}
      <Button variant="secondary" onclick={() => (dialogWithFooterVisible = false)}>
        Cancel
      </Button>
      <Button variant="secondary" onclick={() => (dialogWithFooterVisible = false)}>
        Post as new message
      </Button>
      <Button defaultAction onclick={() => (dialogWithFooterVisible = false)}>
        <span class="iconify icon-[uil--comment-alt-lines]"></span>
        Continue in thread
      </Button>
    {/snippet}
  </Dialog>
</Story>
