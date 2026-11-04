<!-- @component
An accessible icon action for preview toolbars. The owner supplies the localized
label and handles the action. Clicks do not activate an enclosing card.
Disabled state, toggle state, and description references also apply in sheets.
-->
<script lang="ts">
  import MenuItem from './MenuItem.svelte';
  import { hasPreviewActionSheet, usePreviewActionSheet } from './previewActionSheetContext';

  let {
    label,
    icon,
    onclick,
    ...attributes
  }: {
    onclick?: (event: MouseEvent) => void;
    disabled?: boolean;
    'aria-pressed'?: boolean;
    'aria-describedby'?: string;
    /** Accessible name and tooltip. */
    label: string;
    /** Iconify utility class for the decorative icon. */
    icon: string;
  } = $props();

  const closeSheet = hasPreviewActionSheet() ? usePreviewActionSheet() : undefined;

  function activate(event: MouseEvent) {
    event.stopPropagation();
    closeSheet?.();
    onclick?.(event);
  }
</script>

{#if closeSheet}
  <MenuItem
    {icon}
    disabled={attributes.disabled}
    pressed={attributes['aria-pressed']}
    describedBy={attributes['aria-describedby']}
    onclick={activate}>{label}</MenuItem
  >
{:else}
  <button
    type="button"
    {...attributes}
    class="btn-secondary attachment-action-button"
    aria-label={label}
    title={label}
    onclick={activate}
  >
    <span class={['iconify shrink-0 text-sm', icon]} aria-hidden="true"></span>
  </button>
{/if}
