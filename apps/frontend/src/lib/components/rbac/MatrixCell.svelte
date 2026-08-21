<!--
@component

A single cell in the permission matrix. Combines two pieces of information:

  - **inherited**: the resolved baseline from tiers above (faded color)
  - **override**: the explicit override at this tier (saturated color)

By default, click cycles the override through `neutral → allow → deny → neutral`. The
inherited indicator persists faded behind the override (so you can see what
the role would do without the override at this scope).

In `binary` mode the cell exposes only enabled and disabled. Callers can lock
an inherited grant so it can only be changed at the broader source scope.

While a change is being saved, the state icon is replaced with a spinner and
the cell is temporarily non-interactive.

Permission ceilings use a lock only when the cell is fully inert. A configured
grant that remains removable uses a warning marker instead, so the lock never
advertises a clickable control.

When the permission is not applicable to the role at this scope (e.g. a
room-only permission queried at instance scope), pass `applicable={false}`
to render an inert "—" cell with an explanation tooltip.
-->
<script lang="ts">
  type State = 'allow' | 'deny' | 'neutral';

  let {
    override,
    inherited = 'neutral',
    applicable = true,
    disabled = false,
    locked = false,
    allowBlocked = false,
    ceilingBlocked = false,
    decisionMode = 'tri-state',
    updating = false,
    ariaLabel,
    title,
    onCycle
  }: {
    override: State;
    inherited?: State;
    applicable?: boolean;
    disabled?: boolean;
    /** Keep an inherited state visible while making the cell fully inert. */
    locked?: boolean;
    /** Skip the allow state when a delegation ceiling makes it invalid. */
    allowBlocked?: boolean;
    /** Marks a configured allow that is dormant under a delegation ceiling. */
    ceilingBlocked?: boolean;
    decisionMode?: 'tri-state' | 'binary';
    updating?: boolean;
    ariaLabel: string;
    title?: string;
    onCycle: (next: State) => void;
  } = $props();

  function nextState(): State {
    if (decisionMode === 'binary') return visual === 'allow' ? 'neutral' : 'allow';
    if (override === 'neutral') return allowBlocked ? 'deny' : 'allow';
    if (override === 'allow') return 'deny';
    return 'neutral';
  }

  function handleClick() {
    if (
      disabled ||
      locked ||
      updating ||
      !applicable ||
      (decisionMode === 'binary' && allowBlocked && visual !== 'allow')
    )
      return;
    onCycle(nextState());
  }

  // The cell is colored by the *override* when present, otherwise by the
  // inherited baseline (so a row's effective state is visible at a glance,
  // matching the editor's "permission name reflects effective state" rule).
  const visual = $derived(override !== 'neutral' ? override : inherited);
  const isOverride = $derived(override !== 'neutral');
  const interactionDisabled = $derived(
    disabled || locked || (decisionMode === 'binary' && allowBlocked && visual !== 'allow')
  );
  const interactive = $derived(!interactionDisabled && !updating);

  // Overrides use a solid semantic fill and its contrast-safe foreground.
  // Inherited states use a quiet tint; neutral uses the surface ladder.
  const overrideClasses: Record<State, string> = {
    allow: 'bg-success text-on-success',
    deny: 'bg-danger text-on-danger',
    // Unreachable — neutral isn't an override state, but keep a value for type safety.
    neutral: ''
  };
  const inheritedClasses: Record<State, string> = {
    allow: 'bg-success/15 text-success/85',
    deny: 'bg-danger/15 text-danger/85',
    neutral: 'bg-surface-emphasized/60 text-muted/60'
  };

  const surfaceClasses = $derived.by(() => {
    const base = ceilingBlocked
      ? isOverride
        ? 'bg-warning text-on-warning'
        : 'bg-warning/20 text-warning'
      : isOverride
        ? overrideClasses[visual]
        : inheritedClasses[visual];

    if (!interactive) return base;
    const hover = ceilingBlocked
      ? isOverride
        ? 'hover:bg-warning/90'
        : 'hover:bg-warning/30'
      : visual === 'allow'
        ? isOverride
          ? 'hover:bg-success/90'
          : 'hover:bg-success/25'
        : visual === 'deny'
          ? isOverride
            ? 'hover:bg-danger/90'
            : 'hover:bg-danger/25'
          : 'hover:bg-surface-strong/80';
    return `${base} ${hover}`;
  });

  const icon = $derived.by(() => {
    if (visual === 'allow') return 'icon-[uil--check]';
    if (visual === 'deny' && decisionMode === 'tri-state') return 'icon-[uil--times]';
    return 'icon-[uil--minus]';
  });

</script>

{#if !applicable}
  <span
    class="inline-flex h-10 w-10 items-center justify-center text-xs text-muted/30"
    {title}
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
      updating ? 'bg-action/15 ring-2 ring-action/40 ring-inset' : '',
      disabled && !locked && !allowBlocked ? 'opacity-60' : ''
    ]}
    disabled={interactionDisabled || updating}
    {title}
    aria-label={ariaLabel}
    aria-busy={updating || undefined}
    aria-pressed={decisionMode === 'binary' ? visual === 'allow' : isOverride}
    onclick={handleClick}
  >
    <span
      class={[
        'inline-flex h-5 w-5 items-center justify-center rounded-md transition-[background-color,color]',
        surfaceClasses
      ]}
    >
      {#if updating}
        <span class="iconify icon-[uil--spinner] h-4 w-4 animate-spin" aria-hidden="true"></span>
      {:else}
        <span class={['iconify h-3 w-3', icon]}></span>
      {/if}
    </span>
    {#if (locked || (allowBlocked && interactionDisabled)) && !updating}
      <span
        class="iconify absolute top-0.5 right-0.5 icon-[uil--lock] h-3 w-3 text-warning"
        aria-hidden="true"
      ></span>
    {:else if (allowBlocked || ceilingBlocked) && !updating}
      <span
        class="iconify absolute top-0.5 right-0.5 icon-[uil--exclamation-triangle] h-3 w-3 text-warning"
        aria-hidden="true"
      ></span>
    {/if}
  </button>
{/if}
