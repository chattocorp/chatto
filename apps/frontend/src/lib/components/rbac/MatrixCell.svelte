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
  import { MatrixCellButton, type MatrixCellTone } from '$lib/components/matrix';

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
  const displayLocked = $derived(locked || (allowBlocked && interactionDisabled));
  const icon = $derived.by(() => {
    if (visual === 'allow') return 'icon-[uil--check]';
    if (visual === 'deny' && decisionMode === 'tri-state') return 'icon-[uil--times]';
    return 'icon-[uil--minus]';
  });
  const tone = $derived.by<MatrixCellTone>(() => {
    if (ceilingBlocked) return 'warning';
    if (visual === 'allow') return 'success';
    if (visual === 'deny') return 'danger';
    return 'neutral';
  });
</script>

<MatrixCellButton
  {tone}
  explicit={isOverride}
  {icon}
  loading={updating}
  disabled={interactionDisabled}
  locked={displayLocked}
  warning={!displayLocked && (allowBlocked || ceilingBlocked)}
  {applicable}
  pressed={decisionMode === 'binary' ? visual === 'allow' : isOverride}
  {ariaLabel}
  {title}
  onActivate={handleClick}
/>
