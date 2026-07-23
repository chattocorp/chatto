<!--
@component

Tiny test double for form Button used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    variant,
    disabled = false,
    loading = false,
    onclick
  }: {
    children: Snippet;
    variant?: string;
    disabled?: boolean;
    loading?: boolean;
    onclick?: (e: MouseEvent) => void;
  } = $props();
</script>

<button
  type="button"
  data-variant={variant}
  disabled={disabled || loading}
  aria-busy={loading || undefined}
  {onclick}
>
  {@render children()}
</button>
