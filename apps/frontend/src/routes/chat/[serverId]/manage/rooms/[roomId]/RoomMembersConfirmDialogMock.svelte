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
    onclose: () => void;
  } = $props();
</script>

<dialog open aria-label={title}>
  <div>{@render children()}</div>
  <button type="button" onclick={onclose}>Cancel</button>
  <button type="button" disabled={loading} onclick={onconfirm}>{actionLabel}</button>
</dialog>
