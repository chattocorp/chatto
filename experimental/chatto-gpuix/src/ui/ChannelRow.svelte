<script lang="ts">
  import Icon from './Icon.svelte';
  let {
    label,
    selected = false,
    direct = false,
    unread = 0,
    onclick,
    testid
  }: {
    label: string;
    selected?: boolean;
    direct?: boolean;
    unread?: number;
    onclick: () => void;
    testid?: string;
  } = $props();
</script>

<div
  class={['row', { selected }]}
  role="button"
  aria-label={selected ? `${label}, current conversation` : label}
  tabindex="0"
  hitbox="self"
  {testid}
  {onclick}
  onkeydown={(event) => {
    if (event.key === 'enter' || event.key === 'space') onclick();
  }}
>
  <Icon name={direct ? 'message' : 'hash'} size={17} accent={selected} />
  <text class="label">{label}</text>
  {#if unread}<text class="badge">{unread}</text>{/if}
</div>

<style>
  .row {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-10);
    height: var(--size-32);
    padding: 0 var(--size-9);
    border-radius: var(--radius-control);
    cursor: pointer;
    color: var(--color-text);
  }
  .row:hover {
    background-color: var(--color-surface);
  }
  .row.selected {
    background-color: var(--color-selected);
    color: var(--color-text-top);
  }
  .row:focus-visible {
    background-color: var(--color-selected);
  }
  .label {
    flex-grow: 1;
    min-width: 0;
    font-size: var(--text-body);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .badge {
    background-color: var(--color-action);
    color: #05232c;
    border-radius: var(--size-8);
    padding: var(--size-1) var(--size-5);
    font-size: var(--text-badge);
    font-weight: 600;
  }
</style>
