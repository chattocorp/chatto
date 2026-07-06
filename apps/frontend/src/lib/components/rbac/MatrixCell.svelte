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

  // Override = saturated gradient + white icon (poppy).
  // Inherited = lighter gradient (recognisable but quiet).
  // Neutral = barely-there surface gradient (clickable hint).
  // Drop shadows are intentionally absent — the design language uses
  // gradients + soft rings for depth, not shadows.
  const overrideClasses: Record<State, string> = {
    allow:
      'bg-gradient-to-br from-success/65 to-success/95 text-white hover:from-success/75 hover:to-success',
    deny: 'bg-gradient-to-br from-danger/65 to-danger/95 text-white hover:from-danger/75 hover:to-danger',
    // Unreachable — neutral isn't an override state, but keep a value for type safety.
    neutral: ''
  };
  const inheritedClasses: Record<State, string> = {
    allow:
      'bg-gradient-to-br from-success/15 to-success/30 text-success/85 hover:from-success/25 hover:to-success/40',
    deny: 'bg-gradient-to-br from-danger/15 to-danger/30 text-danger/85 hover:from-danger/25 hover:to-danger/40',
    neutral:
      'bg-gradient-to-br from-surface-200/40 to-surface-300/60 text-muted/60 hover:from-surface-200/60 hover:to-surface-300/80'
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
        'inline-flex h-5 w-5 items-center justify-center rounded-md transition-[background-color,color,box-shadow,--tw-gradient-from,--tw-gradient-to]',
        surfaceClasses
      ]}
    >
      <span class={['iconify h-3 w-3', icon]}></span>
    </span>
  </button>
{/if}
