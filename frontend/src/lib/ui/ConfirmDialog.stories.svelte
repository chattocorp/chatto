<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ConfirmDialog from './ConfirmDialog.svelte';

  const { Story } = defineMeta({
    title: 'UI/ConfirmDialog',
    component: ConfirmDialog,
    tags: ['autodocs']
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

<Story name="Danger (default)" asChild>
  <button class="btn-primary" onclick={() => (dangerVisible = true)}>Open danger dialog</button>

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

<Story name="Warning tone" asChild>
  <button class="btn-primary" onclick={() => (warningVisible = true)}>Open warning dialog</button>

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

<Story name="Info tone (non-destructive)" asChild>
  <button class="btn-primary" onclick={() => (infoVisible = true)}>Open info dialog</button>

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

<Story name="Loading state" asChild>
  <button class="btn-primary" onclick={() => (loadingVisible = true)}>Open loading dialog</button>

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
