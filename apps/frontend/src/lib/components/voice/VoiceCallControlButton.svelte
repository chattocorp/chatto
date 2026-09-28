<!--
@component

Shared icon button for the call panel and persistent current-user call controls.
Keeps labels, pending state, and icon presentation consistent across both surfaces.
-->
<script lang="ts">
  import type { MouseEventHandler } from 'svelte/elements';

  let {
    label,
    testId,
    icon,
    class: className,
    iconClass,
    pending = false,
    disabled = false,
    pressed,
    onclick
  }: {
    label: string;
    testId: string;
    icon: string;
    class: string;
    iconClass?: string;
    pending?: boolean;
    disabled?: boolean;
    /**
     * State of an on/off toggle. Keep `label` constant for toggles, so screen
     * readers announce the action together with its pressed state.
     */
    pressed?: boolean;
    onclick: MouseEventHandler<HTMLButtonElement>;
  } = $props();
</script>

<button
  type="button"
  class={className}
  title={label}
  aria-label={label}
  data-testid={testId}
  {onclick}
  disabled={pending || disabled}
  aria-busy={pending || undefined}
  aria-pressed={pressed}
>
  <span
    class={['iconify', iconClass, pending ? 'icon-[uil--spinner] animate-spin' : icon]}
    aria-hidden="true"
  ></span>
</button>
