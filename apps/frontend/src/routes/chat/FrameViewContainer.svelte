<!--
@component

Renders the shallow-routed global modal that fills the app frame. The root
layout places it inside the app frame; `ModalContainer` renders all other
global modals as dialogs. The view stays mounted while a dialog opens above it.
-->
<script lang="ts">
  import { page } from '$app/state';
  import type { FrameViewModal } from '$lib/modal';
  import AddServerView from './modals/AddServerView.svelte';

  let {
    frameView,
    opener = null
  }: {
    frameView: FrameViewModal;
    /** The element that had focus before the view opened. */
    opener?: HTMLElement | null;
  } = $props();

  /** Go back only while the view is the current entry, not below a dialog. */
  function close() {
    if (page.state.modal?.type === frameView.type) history.back();
  }
</script>

<!-- History navigation creates new state objects; the view type is the identity. -->
{#key frameView.type}
  {#if frameView.type === 'addServer'}
    <AddServerView onclose={close} {opener} />
  {/if}
{/key}
