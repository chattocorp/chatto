<script lang="ts">
  import type { Snippet } from 'svelte';
  let { children, testid }: { children: Snippet; testid?: string } = $props();
</script>

<div class="card" {testid}>{@render children()}</div>

<style>
  .card {
    display: flex;
    flex-direction: column;
    gap: var(--size-8);
    width: var(--size-430);
    max-width: 100%;
    padding: var(--size-10);
    background-color: var(--color-surface);
    border-width: var(--size-1);
    border-color: #353535;
    border-radius: var(--radius-panel);
  }
</style>
