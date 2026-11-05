<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon, { type IconName } from './Icon.svelte';
  /** One header row for every pane. Actions keep their space; long text truncates. */
  let {
    title,
    description,
    icon,
    actions,
    testid
  }: {
    title: string;
    description?: string | null;
    icon?: IconName;
    actions?: Snippet;
    testid?: string;
  } = $props();
</script>

<div class="pane-header" {testid}>
  {#if icon}<Icon name={icon} size={20} />{/if}
  <div class="text">
    <text class="title">{title}</text>
    {#if description}<text class="description">{description}</text>{/if}
  </div>
  {#if actions}<div class="actions">{@render actions()}</div>{/if}
</div>

<style>
  .pane-header {
    display: flex;
    flex-direction: row;
    align-items: center;
    flex-grow: 0;
    flex-shrink: 0;
    height: var(--pane-header-height);
    min-height: var(--pane-header-height);
    padding: 0 var(--pane-header-padding);
    gap: var(--pane-header-gap);
    border-bottom-width: var(--size-1);
    border-color: var(--color-border);
    background-color: var(--color-background);
  }
  .text {
    display: flex;
    flex-direction: row;
    align-items: center;
    flex-grow: 1;
    min-width: 0;
    gap: var(--pane-header-gap);
    overflow: hidden;
  }
  .title {
    font-size: var(--text-heading);
    font-weight: 600;
    color: var(--color-text-top);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
    flex-shrink: 1;
  }
  .description {
    font-size: var(--text-body);
    color: var(--color-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
    flex-shrink: 1;
  }
  .actions {
    display: flex;
    flex-direction: row;
    align-items: center;
    flex-shrink: 0;
    gap: var(--size-4);
  }
</style>
