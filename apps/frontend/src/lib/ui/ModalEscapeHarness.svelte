<!-- @component Exercises modal keyboard ownership with a background pane. -->
<script lang="ts">
  import Dialog from './Dialog.svelte';
  import Combobox from './form/Combobox.svelte';

  let { items = [] }: { items?: string[] } = $props();
  let paneOpen = $state(true);
  let visible = $state(true);
</script>

<svelte:window
  onkeydown={(event) => {
    if (event.key === 'Escape' && !event.defaultPrevented) {
      event.preventDefault();
      paneOpen = false;
    }
  }}
/>

{#if paneOpen}<p>Background pane</p>{/if}
<Dialog bind:visible title="Invite members">
  <Combobox
    id="recipient"
    label="Recipient"
    {items}
    getValue={(item) => item}
    getLabel={(item) => item}
  />
</Dialog>
