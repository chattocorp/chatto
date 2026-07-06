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
    onconfirm
  }: {
    children: Snippet;
    title: string;
    actionLabel: string;
    onconfirm: () => void;
  } = $props();
</script>

<dialog open aria-label={title}>
  <div>{@render children()}</div>
  <button type="submit" onclick={onconfirm}>{actionLabel}</button>
</dialog>
