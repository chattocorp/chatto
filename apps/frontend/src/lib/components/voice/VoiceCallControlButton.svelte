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
    onclick
  }: {
    label: string;
    testId: string;
    icon: string;
    class: string;
    iconClass?: string;
    pending?: boolean;
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
  disabled={pending}
  aria-busy={pending || undefined}
>
  <span
    class={['iconify', iconClass, pending ? 'animate-spin icon-[uil--spinner]' : icon]}
    aria-hidden="true"
  ></span>
</button>
