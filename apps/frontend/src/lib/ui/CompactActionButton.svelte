<!--
@component

A standalone compact action with a flat resting surface and a bevel on hover
or keyboard focus. Owns the 28 px height, icon sizing, and disabled treatment.
Use touchFriendly to enlarge targets and icons when touch input is available.
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
    touchFriendly = false,
    ...attributes
  }: HTMLButtonAttributes & {
    label: string;
    children: Snippet;
    wrapperClass?: ClassValue;
    /** Use 44 px targets and 20 px icons when a touch pointer is available. */
    touchFriendly?: boolean;
  } = $props();
</script>

<div
  class={[
    'pill-button-group pill-button-group-compact w-auto shrink-0 pill-button-group-hover-bevel',
    touchFriendly && 'pill-button-group-touch',
    wrapperClass
  ]}
>
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
