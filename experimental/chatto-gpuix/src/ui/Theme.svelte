<script lang="ts">
  import type { Snippet } from 'svelte';
  import { applyTheme } from './theme.ts';
  let { children }: { children: Snippet } = $props();
  applyTheme();
</script>

<div class="theme">{@render children()}</div>

<style>
  .theme {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    font-family: var(--font-sans);
    font-size: 14px;
    color: var(--color-text);
    background-color: var(--color-background);
  }
</style>
