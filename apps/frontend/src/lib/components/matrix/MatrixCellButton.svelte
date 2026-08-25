<!--
@component

Presentational 40-pixel matrix cell control. The consumer owns the domain
state machine and supplies the semantic tone, icon, presentation, and
activation callback.
-->
<script lang="ts">
  export type MatrixCellTone = 'neutral' | 'action' | 'success' | 'warning' | 'danger';
  export type MatrixCellVariant = 'badge' | 'icon';

  let {
    tone = 'neutral',
    explicit = false,
    variant = 'badge',
    icon,
    loading = false,
    disabled = false,
    locked = false,
    warning = false,
    inheritedMarker = false,
    applicable = true,
    pressed,
    ariaLabel,
    title,
    onActivate
  }: {
    tone?: MatrixCellTone;
    explicit?: boolean;
    variant?: MatrixCellVariant;
    icon: string;
    loading?: boolean;
    disabled?: boolean;
    locked?: boolean;
    warning?: boolean;
    inheritedMarker?: boolean;
    applicable?: boolean;
    pressed?: boolean;
    ariaLabel: string;
    title?: string;
    onActivate: () => void;
  } = $props();

  const explicitClasses: Record<MatrixCellTone, string> = {
    neutral: 'bg-neutral-action text-on-neutral-action',
    action: 'bg-action text-on-action',
    success: 'bg-success text-on-success',
    warning: 'bg-warning text-on-warning',
    danger: 'bg-danger text-on-danger'
  };
  const inheritedClasses: Record<MatrixCellTone, string> = {
    neutral: 'bg-surface-emphasized/60 text-muted/70',
    action: 'bg-action/15 text-action/85',
    success: 'bg-success/15 text-success/85',
    warning: 'bg-warning/20 text-warning',
    danger: 'bg-danger/15 text-danger/85'
  };
  const iconClasses: Record<MatrixCellTone, string> = {
    neutral: 'text-text',
    action: 'text-action',
    success: 'text-success',
    warning: 'text-warning',
    danger: 'text-danger'
  };
  const interactive = $derived(applicable && !disabled && !locked && !loading);
  const hoverClass = $derived.by(() => {
    if (!interactive) return '';
    if (!explicit) {
      return {
        neutral: 'hover:bg-surface-strong/80',
        action: 'hover:bg-action/25',
        success: 'hover:bg-success/25',
        warning: 'hover:bg-warning/30',
        danger: 'hover:bg-danger/25'
      }[tone];
    }
    return {
      neutral: 'hover:bg-neutral-action/90',
      action: 'hover:bg-action/90',
      success: 'hover:bg-success/90',
      warning: 'hover:bg-warning/90',
      danger: 'hover:bg-danger/90'
    }[tone];
  });
  const surfaceClasses = $derived.by(() => {
    if (variant === 'icon') {
      return `${loading ? 'text-action' : iconClasses[tone]} ${explicit ? '' : 'opacity-40'}`;
    }
    return `${explicit ? explicitClasses[tone] : inheritedClasses[tone]} ${hoverClass}`;
  });
</script>

{#if !applicable}
  <span
    class="inline-flex h-10 w-10 items-center justify-center text-xs text-muted/30"
    {title}
    role="img"
    aria-label={ariaLabel}
  >
    —
  </span>
{:else}
  <button
    type="button"
    class={[
      'relative inline-flex h-10 w-10 items-center justify-center rounded-md transition-[scale]',
      interactive ? 'cursor-pointer active:scale-[0.96]' : 'cursor-not-allowed',
      loading && variant === 'badge' ? 'bg-action/15 ring-2 ring-action/40 ring-inset' : '',
      disabled && !locked ? 'opacity-60' : ''
    ]}
    disabled={disabled || locked}
    {title}
    aria-label={ariaLabel}
    aria-busy={loading || undefined}
    aria-disabled={!interactive || undefined}
    aria-pressed={pressed}
    onclick={() => {
      if (interactive) onActivate();
    }}
  >
    <span
      class={[
        'inline-flex h-5 w-5 items-center justify-center transition-[background-color,color,opacity]',
        variant === 'badge' ? 'rounded-md' : '',
        surfaceClasses
      ]}
    >
      {#if loading}
        <span
          class={[
            'iconify icon-[uil--spinner] animate-spin',
            variant === 'icon' ? 'h-5 w-5' : 'h-4 w-4'
          ]}
          aria-hidden="true"
        ></span>
      {:else}
        <span
          class={['iconify', variant === 'icon' ? 'h-5 w-5' : 'h-3 w-3', icon]}
          aria-hidden="true"
        ></span>
      {/if}
    </span>
    {#if locked && !loading}
      <span
        class="iconify absolute end-0.5 top-0.5 icon-[uil--lock] h-3 w-3 text-warning"
        aria-hidden="true"
      ></span>
    {:else if warning && !loading}
      <span
        class="iconify absolute end-0.5 top-0.5 icon-[uil--exclamation-triangle] h-3 w-3 text-warning"
        aria-hidden="true"
      ></span>
    {:else if inheritedMarker && !loading}
      <span
        class="iconify icon-[uil--link] absolute end-0.5 top-0.5 h-3 w-3 rounded-full bg-background text-muted ring-1 ring-border"
        aria-hidden="true"
      ></span>
    {/if}
  </button>
{/if}
