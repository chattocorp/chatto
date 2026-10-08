<!--
@component

Compact metric card for countable operational facts. Use it for values that
benefit from quick comparison across a dashboard row. The tone communicates
the category or health of the metric; it is not decorative.
-->
<script lang="ts">
  type Color = 'action' | 'success' | 'warning' | 'danger';

  let {
    value,
    label,
    icon,
    color = 'action',
    subtitle
  }: {
    value: string | number;
    label: string;
    icon: string;
    color?: Color;
    subtitle?: string;
  } = $props();

  const colorClasses: Record<Color, { bg: string; text: string }> = {
    action: { bg: 'bg-action/10', text: 'text-action' },
    success: { bg: 'bg-success/10', text: 'text-success' },
    warning: { bg: 'bg-warning/10', text: 'text-warning' },
    danger: { bg: 'bg-danger/10', text: 'text-danger' }
  };
</script>

<div class="flex min-w-0 flex-col gap-3 panel-shell p-5">
  <div class="flex items-center gap-2.5">
    <div class="flex rounded-md p-1.5 {colorClasses[color].bg}">
      <span aria-hidden="true" class="{icon} text-lg {colorClasses[color].text}"></span>
    </div>
    <div class="truncate text-sm text-muted">{label}</div>
  </div>
  <div class="min-w-0">
    <div class="truncate text-2xl font-bold tabular-nums sm:text-3xl" title={String(value)}>
      {value}
    </div>
    {#if subtitle}
      <div class="mt-0.5 truncate text-xs text-muted" title={subtitle}>{subtitle}</div>
    {/if}
  </div>
</div>
