<!--
@component

Tiny test double for Dialog used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    footer,
    title
  }: {
    children: Snippet;
    footer?: Snippet;
    title: string;
  } = $props();
</script>

<dialog open aria-label={title}>
  <div>{@render children()}</div>
  {#if footer}
    <footer>{@render footer()}</footer>
  {/if}
</dialog>
