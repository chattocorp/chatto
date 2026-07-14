<!--
@component

A single cell in the permission matrix. Combines two pieces of information:

  - **inherited**: the resolved baseline from tiers above (faded color)
  - **override**: the explicit override at this tier (saturated color)

Click cycles the override through `neutral → allow → deny → neutral`. The
inherited indicator persists faded behind the override (so you can see what
the role would do without the override at this scope).

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
    updating = false,
    ariaLabel,
    title,
    onCycle
  }: {
    override: State;
    inherited?: State;
    applicable?: boolean;
    disabled?: boolean;
    updating?: boolean;
    ariaLabel: string;
    title?: string;
    onCycle: (next: State) => void;
  } = $props();

  function nextState(): State {
    if (override === 'neutral') return 'allow';
    if (override === 'allow') return 'deny';
    return 'neutral';
  }

  function handleClick() {
    if (disabled || !applicable) return;
    onCycle(nextState());
  }

  // The cell is colored by the *override* when present, otherwise by the
  // inherited baseline (so a row's effective state is visible at a glance,
  // matching the editor's "permission name reflects effective state" rule).
  const visual = $derived(override !== 'neutral' ? override : inherited);
  const isOverride = $derived(override !== 'neutral');

  // Overrides use a solid semantic fill and its contrast-safe foreground.
  // Inherited states use a quiet tint; neutral uses the surface ladder.
  const overrideClasses: Record<State, string> = {
    allow: 'bg-success text-on-success hover:bg-success/90',
    deny: 'bg-danger text-on-danger hover:bg-danger/90',
    // Unreachable — neutral isn't an override state, but keep a value for type safety.
    neutral: ''
  };
  const inheritedClasses: Record<State, string> = {
    allow: 'bg-success/15 text-success/85 hover:bg-success/25',
    deny: 'bg-danger/15 text-danger/85 hover:bg-danger/25',
    neutral: 'bg-surface-emphasized/60 text-muted/60 hover:bg-surface-strong/80'
  };

  const surfaceClasses = $derived(isOverride ? overrideClasses[visual] : inheritedClasses[visual]);

  const icon = $derived.by(() => {
    if (visual === 'allow') return 'uil--check';
    if (visual === 'deny') return 'uil--times';
    return 'uil--minus';
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
      'inline-flex h-10 w-10 cursor-pointer items-center justify-center rounded-md transition-[scale] active:scale-[0.96]',
      updating ? 'animate-pulse' : '',
      disabled ? 'cursor-not-allowed opacity-60' : ''
    ]}
    {disabled}
    {title}
    aria-label={ariaLabel}
    aria-pressed={isOverride}
    onclick={handleClick}
  >
    <span
      class={[
        'inline-flex h-5 w-5 items-center justify-center rounded-md transition-[background-color,color]',
        surfaceClasses
      ]}
    >
      <span class={['iconify h-3 w-3', icon]}></span>
    </span>
  </button>
{/if}
