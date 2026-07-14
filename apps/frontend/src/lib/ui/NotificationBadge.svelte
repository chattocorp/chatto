<script lang="ts">
  type Color = 'warning' | 'action' | 'muted';

  let {
    count,
    color = 'warning',
    overlay = false,
    class: className,
    testid
  }: {
    count: number;
    color?: Color;
    overlay?: boolean;
    class?: string;
    testid?: string;
  } = $props();

  const colorClasses = {
    warning: 'bg-warning text-on-warning',
    action: 'bg-action text-on-action',
    muted: 'bg-muted text-background'
  } as const;

  const label = $derived(count > 99 ? '99+' : String(Math.max(0, count)));
</script>

<span
  class={[
    'inline-flex shrink-0 items-center justify-center px-1.5 text-[10px] leading-none font-semibold tabular-nums',
    overlay
      ? 'h-5 min-w-5 rounded-sm px-1 shadow-sm ring-2 ring-background'
      : 'min-h-5 min-w-5 rounded',
    colorClasses[color],
    className
  ]}
  aria-hidden="true"
  data-testid={testid}>{label}</span
>
