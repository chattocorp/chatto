<!--
@component

Renders the shallow-routed global modals that fill the app frame. The root
layout places it inside the app frame; `ModalContainer` renders all other
global modals as dialogs.
-->
<script lang="ts">
  import { page } from '$app/state';
  import { isFrameViewModal, type ChatModal } from '$lib/modal';
  import AddServerView from './modals/AddServerView.svelte';

  const modal = $derived(page.state.modal);

  function closeModalFor(expectedModal: ChatModal) {
    return () => {
      if (page.state.modal === expectedModal) history.back();
    };
  }
</script>

{#if isFrameViewModal(modal)}
  {#key modal}
    {#if modal.type === 'addServer'}
      <AddServerView onclose={closeModalFor(modal)} />
    {/if}
  {/key}
{/if}
