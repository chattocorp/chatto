<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  let { label, children }: { label: string; children: Snippet } = $props();
  let collapsed = $state(false);
  function toggle() {
    collapsed = !collapsed;
  }
</script>

<div class="section">
  <div
    class="heading"
    role="button"
    aria-label={label}
    aria-expanded={!collapsed}
    tabindex="0"
    hitbox="self"
    onclick={toggle}
    onkeydown={(event) => {
      if (event.key === 'enter' || event.key === 'space') toggle();
    }}
  >
    <Icon name={collapsed ? 'chevronRight' : 'chevron'} size={14} /><text>{label}</text>
  </div>
  {#if !collapsed}{@render children()}{/if}
</div>

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--size-2);
    padding: var(--size-12) var(--size-7);
    border-bottom-width: var(--size-1);
    border-color: var(--color-border);
  }
  .heading {
    display: flex;
    align-items: center;
    gap: var(--size-9);
    padding: var(--size-6) var(--size-7) var(--size-10);
    color: var(--color-muted);
    font-size: var(--text-small);
    font-weight: 600;
    cursor: pointer;
  }
  .heading:hover {
    color: var(--color-text-top);
  }
  .heading:focus-visible {
    background-color: var(--color-selected);
  }
</style>
