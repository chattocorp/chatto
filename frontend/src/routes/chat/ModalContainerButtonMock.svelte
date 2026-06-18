<!--
@component

Tiny test double for form Button used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    onclick
  }: {
    children: Snippet;
    onclick?: (e: MouseEvent) => void;
  } = $props();
</script>

<button type="button" {onclick}>{@render children()}</button>
