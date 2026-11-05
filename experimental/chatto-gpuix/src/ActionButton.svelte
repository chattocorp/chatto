<script lang="ts">
  import type { GpuixEvent } from 'gpuix-svelte';
  import Icon, { type IconName } from './ui/Icon.svelte';

  let {
    label,
    onclick,
    testid,
    primary = false,
    quiet = false,
    icon,
    iconOnly = false,
    selected = false,
    align = 'center',
    disabled = false
  }: {
    label: string;
    onclick: () => void;
    testid?: string;
    primary?: boolean;
    quiet?: boolean;
    icon?: IconName;
    iconOnly?: boolean;
    selected?: boolean;
    align?: 'start' | 'center';
    disabled?: boolean;
  } = $props();

  function activate() {
    if (!disabled) onclick();
  }

  function keydown(event: GpuixEvent) {
    if (event.key === 'enter' || event.key === 'space') activate();
  }
</script>

<div
  class={['button', { primary, quiet, iconOnly, disabled, selected, start: align === 'start' }]}
  role="button"
  aria-label={label}
  tabindex={disabled ? -1 : 0}
  hitbox="self"
  {testid}
  onclick={activate}
  onkeydown={keydown}
>
  {#if icon}<Icon name={icon} size={17} />{/if}
  {#if !iconOnly}<text>{label}</text>{/if}
</div>

<style>
  .button {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    padding: 6px 10px;
    border-radius: var(--radius-control);
    background-color: var(--color-surface-emphasized);
    color: var(--color-text);
    font-size: 13px;
    cursor: pointer;
    flex-shrink: 0;
  }
  .button:hover {
    background-color: #363636;
  }
  .selected {
    background-color: var(--color-selected);
    color: var(--color-text-top);
  }
  .start {
    justify-content: flex-start;
  }
  .primary {
    background-color: var(--color-action);
    color: #042630;
  }
  .primary:hover {
    background-color: #3ac9ec;
  }
  .quiet {
    background-color: transparent;
    color: var(--color-muted);
  }
  .quiet:hover {
    background-color: var(--color-surface);
    color: var(--color-text-top);
  }
  .iconOnly {
    width: 30px;
    height: 30px;
    padding: 5px;
  }
  .button:focus-visible {
    background-color: var(--color-selected);
    border-width: 1px;
    border-color: var(--color-action);
  }
  .disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
