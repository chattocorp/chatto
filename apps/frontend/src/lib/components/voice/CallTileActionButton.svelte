<!--
@component

Compact icon button for call participant tile overlays. Used for local-only
presentation controls such as fullscreening a feed or muting a participant
for the current viewer.
-->
<script lang="ts">
  let {
    icon,
    label,
    active = false,
    testId,
    onclick
  }: {
    icon: string;
    label: string;
    active?: boolean;
    testId?: string;
    onclick?: (event: MouseEvent) => void;
  } = $props();
</script>

<button
  type="button"
  class={[
    'pointer-events-auto flex h-10 w-10 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface-emphasized hover:text-text focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-neutral-action active:scale-[0.96]',
    active && 'bg-surface-emphasized text-text'
  ]}
  title={label}
  aria-label={label}
  data-testid={testId}
  {onclick}
>
  <span class={['iconify text-base', icon]} aria-hidden="true"></span>
</button>
