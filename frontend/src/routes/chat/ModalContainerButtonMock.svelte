<!--
@component

Tiny test double for form Button used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    disabled = false,
    onclick
  }: {
    children: Snippet;
    disabled?: boolean;
    onclick?: (e: MouseEvent) => void;
  } = $props();
</script>

<button type="button" {disabled} {onclick}>{@render children()}</button>
