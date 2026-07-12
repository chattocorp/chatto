<!--
@component

Tiny test double for ConfirmDialog used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    title,
    actionLabel,
    onconfirm,
    onclose
  }: {
    children: Snippet;
    title: string;
    actionLabel: string;
    onconfirm: () => void;
    onclose?: () => void;
  } = $props();
</script>

<dialog open aria-label={title}>
  <div>{@render children()}</div>
  {#if onclose}
    <button type="button" onclick={onclose}>Cancel</button>
  {/if}
  <button type="submit" onclick={onconfirm}>{actionLabel}</button>
</dialog>
