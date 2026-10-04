<!--
@component

A large checkbox tile with an icon, a title, and a description. Use it for a
multi-select choice where each option needs a short explanation. Put the tiles
in a `<fieldset>` with a legend. A selected tile shows a fill and a check
badge. The title is the accessible name of the checkbox; the description and
`disabledReason` describe it. A disabled tile stays focusable, so keyboard and
screen-reader users can reach the `disabledReason` shown below its
description, but it does not change its state.
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';

  let {
    checked = $bindable(false),
    title,
    description,
    icon,
    disabled = false,
    disabledReason,
    onchange,
    class: className
  }: {
    checked?: boolean;
    title: string;
    description: string;
    /** Iconify class, for example `icon-[uil--robot]`. */
    icon: string;
    disabled?: boolean;
    /** Text shown below the description while the tile is disabled. */
    disabledReason?: string;
    onchange?: (checked: boolean) => void;
    /** Layout-only classes such as grid placement. */
    class?: ClassValue;
  } = $props();

  const id = $props.id();
  const showReason = $derived(disabled && !!disabledReason);
</script>

<label
  class={[
    'relative flex items-start gap-3 rounded-lg border option-depth p-3 text-start transition-colors feedback-quick',
    checked ? 'border-action bg-action/10' : 'border-border',
    disabled
      ? 'cursor-not-allowed'
      : ['cursor-pointer', !checked && 'hover:border-input-border hover:bg-surface'],
    'has-focus-visible:outline-2 has-focus-visible:outline-offset-2 has-focus-visible:outline-text',
    className
  ]}
>
  <input
    type="checkbox"
    class="sr-only"
    aria-labelledby="{id}-title"
    aria-describedby={showReason ? `${id}-description ${id}-reason` : `${id}-description`}
    aria-disabled={disabled}
    bind:checked
    onclick={(event) => {
      if (disabled) event.preventDefault();
    }}
    onchange={(event) => onchange?.(event.currentTarget.checked)}
  />
  <span
    class={[
      'flex size-10 shrink-0 items-center justify-center rounded-lg transition-colors feedback-quick',
      checked ? 'bg-action text-on-action' : 'bg-surface text-muted',
      disabled && 'opacity-50'
    ]}
    aria-hidden="true"
  >
    <span class={['iconify text-xl', icon]} aria-hidden="true"></span>
  </span>
  <span class="min-w-0 flex-1 pe-6">
    <span class={['block', disabled && 'opacity-50']}>
      <span
        id="{id}-title"
        class={['block', checked ? 'font-semibold text-text-top' : 'font-medium']}>{title}</span
      >
      <span id="{id}-description" class="block text-sm text-muted">{description}</span>
    </span>
    {#if showReason}
      <span id="{id}-reason" class="mt-1 block text-sm text-text">{disabledReason}</span>
    {/if}
  </span>
  <span
    class={[
      'absolute end-3 top-3 flex size-5 items-center justify-center rounded-full border-2 transition-colors feedback-quick',
      checked ? 'border-action bg-action text-on-action' : 'border-muted',
      disabled && 'opacity-50'
    ]}
    aria-hidden="true"
  >
    {#if checked}
      <span class="iconify icon-[uil--check] text-sm" aria-hidden="true"></span>
    {/if}
  </span>
</label>
