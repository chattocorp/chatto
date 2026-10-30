<!-- @component
An accessible icon action for preview toolbars. The owner supplies the localized
label and handles the action. Clicks do not activate an enclosing card.
Other native button attributes pass through unchanged.
-->
<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  let {
    label,
    icon,
    onclick,
    ...attributes
  }: Omit<HTMLButtonAttributes, 'children' | 'class' | 'aria-label' | 'title'> & {
    /** Accessible name and tooltip. */
    label: string;
    /** Iconify utility class for the decorative icon. */
    icon: string;
  } = $props();
</script>

<button
  type="button"
  {...attributes}
  class="btn-secondary attachment-action-button"
  aria-label={label}
  title={label}
  onclick={(event) => {
    event.stopPropagation();
    onclick?.(event);
  }}
>
  <span class={['iconify shrink-0 text-sm', icon]} aria-hidden="true"></span>
</button>
