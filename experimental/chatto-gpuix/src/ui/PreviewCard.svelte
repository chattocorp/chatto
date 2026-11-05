<script lang="ts">
  import type { Snippet } from 'svelte';
  let { children, testid }: { children: Snippet; testid?: string } = $props();
</script>

<div class="card" {testid}>{@render children()}</div>

<style>
  .card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 430px;
    max-width: 100%;
    padding: 10px;
    background-color: var(--color-surface);
    border-width: 1px;
    border-color: #353535;
    border-radius: var(--radius-panel);
  }
</style>
