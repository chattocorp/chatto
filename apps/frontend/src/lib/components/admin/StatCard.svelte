<script lang="ts">
  type Color = 'primary' | 'success' | 'warning' | 'danger';

  let {
    value,
    label,
    icon,
    color = 'primary',
    subtitle
  }: {
    value: string | number;
    label: string;
    icon: string;
    color?: Color;
    subtitle?: string;
  } = $props();

  const colorClasses: Record<Color, { bg: string; text: string }> = {
    primary: { bg: 'bg-primary/10', text: 'text-primary' },
    success: { bg: 'bg-success/10', text: 'text-success' },
    warning: { bg: 'bg-warning/10', text: 'text-warning' },
    danger: { bg: 'bg-danger/10', text: 'text-danger' }
  };
</script>

<div class="panel-shell p-6">
  <div class="flex items-center gap-4">
    <div class="rounded-lg p-3 {colorClasses[color].bg}">
      <span class="{icon} text-2xl {colorClasses[color].text}"></span>
    </div>
    <div class="min-w-0">
      <div class="text-3xl font-bold">{value}</div>
      <div class="text-sm text-muted">{label}</div>
      {#if subtitle}
        <div class="text-xs text-muted">{subtitle}</div>
      {/if}
    </div>
  </div>
</div>
