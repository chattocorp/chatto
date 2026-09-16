<!--
@component

A standalone compact action with a flat resting surface and a bevel on hover
or keyboard focus. Owns the 28 px height, icon sizing, and disabled treatment.
Use PillButtonGroup directly for joined toolbars.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes, ClassValue } from 'svelte/elements';

  let {
    label,
    children,
    class: className,
    wrapperClass,
    ...attributes
  }: HTMLButtonAttributes & {
    label: string;
    children: Snippet;
    wrapperClass?: ClassValue;
  } = $props();
</script>

<div class={['pill-button-group pill-button-group-compact pill-button-group-hover-bevel w-auto shrink-0', wrapperClass]}>
  <button
    type="button"
    aria-label={label}
    title={label}
    {...attributes}
    class={['pill-button', className]}
  >
    {@render children()}
  </button>
</div>
