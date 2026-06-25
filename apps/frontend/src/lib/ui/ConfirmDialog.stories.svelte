<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import ConfirmDialog from './ConfirmDialog.svelte';

  const componentDescription = `
    Use ConfirmDialog when a command needs explicit confirmation before continuing. Prefer danger
    for destructive, warning for disruptive but recoverable, and info for neutral confirmation flows.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/ConfirmDialog',
    component: ConfirmDialog,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  let dangerVisible = $state(false);
  let warningVisible = $state(false);
  let infoVisible = $state(false);
  let loadingVisible = $state(false);
  let loading = $state(false);

  function startLoading() {
    loading = true;
    setTimeout(() => {
      loading = false;
      loadingVisible = false;
    }, 1500);
  }
</script>

<Story
  name="Danger (default)"
  asChild
  parameters={{
    docs: {
      description: { story: 'Default destructive confirmation with a danger action tone.' }
    }
  }}
>
  <Button onclick={() => (dangerVisible = true)}>Open danger dialog</Button>

  <ConfirmDialog
    bind:visible={dangerVisible}
    title="Delete Message"
    actionLabel="Delete"
    actionIcon="iconify uil--trash-alt"
    onconfirm={() => (dangerVisible = false)}
    onclose={() => (dangerVisible = false)}
  >
    Are you sure you want to delete this message? This cannot be undone.
  </ConfirmDialog>
</Story>

<Story
  name="Warning tone"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Warning tone fits disruptive actions that are not permanent deletion.'
      }
    }
  }}
>
  <Button onclick={() => (warningVisible = true)}>Open warning dialog</Button>

  <ConfirmDialog
    bind:visible={warningVisible}
    title="End Call for Everyone"
    tone="warning"
    actionLabel="End Call"
    actionIcon="iconify uil--phone-slash"
    onconfirm={() => (warningVisible = false)}
    onclose={() => (warningVisible = false)}
  >
    This will disconnect every participant from the call. They can rejoin afterwards.
  </ConfirmDialog>
</Story>

<Story
  name="Info tone (non-destructive)"
  asChild
  parameters={{
    docs: {
      description: { story: 'Info tone keeps neutral confirmations from looking destructive.' }
    }
  }}
>
  <Button onclick={() => (infoVisible = true)}>Open info dialog</Button>

  <ConfirmDialog
    bind:visible={infoVisible}
    title="Sign Out"
    tone="info"
    actionLabel="Sign Out"
    actionIcon="iconify uil--signout"
    onconfirm={() => (infoVisible = false)}
    onclose={() => (infoVisible = false)}
  >
    This will disconnect all instances and sign you out. Your accounts on each instance are not
    affected.
  </ConfirmDialog>
</Story>

<Story
  name="Loading state"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Loading state keeps the dialog open and disables duplicate confirmation.'
      }
    }
  }}
>
  <Button onclick={() => (loadingVisible = true)}>Open loading dialog</Button>

  <ConfirmDialog
    bind:visible={loadingVisible}
    title="Leave Space"
    actionLabel="Leave Space"
    actionIcon="iconify uil--sign-out-alt"
    {loading}
    onconfirm={startLoading}
    onclose={() => (loadingVisible = false)}
  >
    Are you sure you want to leave <strong>Acme Inc.</strong>? You'll lose access to all rooms.
  </ConfirmDialog>
</Story>
