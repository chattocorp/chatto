<script lang="ts">
  type Color = 'warning' | 'primary' | 'muted';

  let {
    color = 'warning',
    overlay = false,
    class: className,
    testid
  }: {
    color?: Color;
    overlay?: boolean;
    class?: string;
    testid?: string;
  } = $props();

  const colorClasses = {
    warning: 'bg-warning',
    primary: 'bg-primary',
    muted: 'bg-muted'
  } as const;
</script>

<span
  class={[
    'shrink-0 rounded-full',
    colorClasses[color],
    overlay ? 'h-3 w-3 shadow-sm ring-2 ring-background' : 'h-2 w-2',
    className
  ]}
  aria-hidden="true"
  data-testid={testid}
></span>
