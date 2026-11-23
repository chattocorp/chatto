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
    active = false,
    onclick,
    ...attributes
  }: Omit<HTMLButtonAttributes, 'children' | 'class' | 'aria-label' | 'title'> & {
    /** Accessible name and tooltip. */
    label: string;
    /** Iconify utility class for the decorative icon. */
    icon: string;
    /**
     * Tints the icon to show that the action's value is set, such as an
     * existing description. The label must also say so. Use `aria-pressed`
     * instead for toggles.
     */
    active?: boolean;
  } = $props();
</script>

<button
  type="button"
  {...attributes}
  class={['attachment-action-button', active && 'text-action not-disabled:hover:text-action']}
  aria-label={label}
  title={label}
  onclick={(event) => {
    event.stopPropagation();
    onclick?.(event);
  }}
>
  <span class={['iconify shrink-0 text-sm', icon]} aria-hidden="true"></span>
</button>
