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
    loading = false,
    onconfirm,
    onclose
  }: {
    children: Snippet;
    title: string;
    actionLabel: string;
    loading?: boolean;
    onconfirm: () => void;
    onclose?: () => void;
  } = $props();
</script>

<dialog open aria-label={title}>
  <div>{@render children()}</div>
  {#if onclose}
    <button type="button" onclick={onclose}>Cancel</button>
  {/if}
  <button
    type="submit"
    disabled={loading}
    aria-busy={loading ? 'true' : undefined}
    onclick={onconfirm}>{actionLabel}</button
  >
</dialog>
