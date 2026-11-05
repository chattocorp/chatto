<script lang="ts">
  import ActionButton from '../ActionButton.svelte';
  import type { IconName } from './Icon.svelte';
  /** A quiet icon action with a standard hitbox and a required accessible name. */
  let {
    label,
    icon,
    onclick,
    disabled = false,
    testid
  }: {
    label: string;
    icon: IconName;
    onclick: () => void;
    disabled?: boolean;
    testid?: string;
  } = $props();
</script>

<ActionButton {label} {icon} {onclick} {disabled} {testid} iconOnly quiet />
