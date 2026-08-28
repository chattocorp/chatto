<!--
@component

Standard one-of-many choice used in settings and preference screens. Place a
set of ChoiceRow components inside an element with `role="radiogroup"` and an
accessible label. The component owns the radio semantics, selected indicator,
disabled state, and typography.
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';

  let {
    label,
    description,
    selected = false,
    disabled = false,
    onclick,
    class: className
  }: {
    label: string;
    description?: string;
    selected?: boolean;
    disabled?: boolean;
    onclick?: (event: MouseEvent) => void;
    /** Layout-only classes such as responsive visibility or width. */
    class?: ClassValue;
  } = $props();
</script>

<button
  type="button"
  role="radio"
  aria-checked={selected}
  {disabled}
  {onclick}
  class={['choice-row', selected && 'choice-row-selected', className]}
>
  <span class={['choice-indicator', selected && 'choice-indicator-selected']} aria-hidden="true">
    {#if selected}
      <span class="choice-indicator-dot"></span>
    {/if}
  </span>
  <span class="min-w-0">
    <span class={['block', selected && 'font-medium']}>{label}</span>
    {#if description}
      <span class="block text-sm text-muted">{description}</span>
    {/if}
  </span>
</button>
