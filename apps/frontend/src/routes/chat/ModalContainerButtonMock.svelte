<!--
@component

Tiny test double for form Button used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    disabled = false,
    loading = false,
    onclick
  }: {
    children: Snippet;
    disabled?: boolean;
    loading?: boolean;
    onclick?: (e: MouseEvent) => void;
  } = $props();
</script>

<button type="button" disabled={disabled || loading} aria-busy={loading || undefined} {onclick}>
  {@render children()}
</button>
