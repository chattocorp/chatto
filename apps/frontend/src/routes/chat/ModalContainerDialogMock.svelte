<!--
@component

Tiny test double for Dialog used by ModalContainer specs.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    footer,
    primaryAction,
    secondaryActions,
    dismissAction,
    title
  }: {
    children: Snippet;
    footer?: Snippet;
    primaryAction?: Snippet;
    secondaryActions?: Snippet;
    dismissAction?: Snippet;
    title: string;
  } = $props();
</script>

<dialog open aria-label={title}>
  <div>{@render children()}</div>
  {@render dismissAction?.()}
  {@render secondaryActions?.()}
  {@render primaryAction?.()}
  {#if footer}
    <footer>{@render footer()}</footer>
  {/if}
</dialog>
