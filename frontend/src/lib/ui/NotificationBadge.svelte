<script lang="ts">
  type Color = 'warning' | 'primary' | 'muted';

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
    warning: 'bg-warning text-white',
    primary: 'bg-primary text-white',
    muted: 'bg-muted text-background'
  } as const;

  const label = $derived(count > 99 ? '99+' : String(Math.max(0, count)));
</script>

<span
  class={[
    'inline-flex shrink-0 items-center justify-center rounded px-1.5 text-[10px] leading-none font-semibold tabular-nums',
    overlay ? 'min-h-4 min-w-4 shadow-sm ring-2 ring-background' : 'min-h-5 min-w-5',
    colorClasses[color],
    className
  ]}
  aria-hidden="true"
  data-testid={testid}
>{label}</span>
