<!--
@component

A single cell in the permission matrix. Combines two pieces of information:

  - **inherited**: the resolved baseline from tiers above (faded color)
  - **override**: the explicit override at this tier (saturated color)

By default, click cycles the override through `neutral → allow → deny → neutral`.
Cells of named roles pass `canDeny={false}`: roles only grant (ADR-116), so they
cycle through `neutral → allow → neutral`. The
inherited indicator persists faded behind the override (so you can see what
the role would do without the override at this scope).

In `binary` mode the cell exposes only enabled and disabled. Callers can lock
an inherited grant so it can only be changed at the broader source scope.

While a change is being saved, the state icon is replaced with a spinner and
the cell is temporarily non-interactive.

Permission ceilings use a lock only when the cell is fully inert. A configured
grant that remains removable uses a warning marker instead, so the lock never
advertises a clickable control.

Pass `effective`, the result that the server computed, to color the cell by the
real result instead of the override. The override still shows as an explicit
marker. `privilegedOnly` marks a result that is allowed only while the account
has privileged mode active.

When the permission is not applicable to the role at this scope (e.g. a
room-only permission queried at instance scope), pass `applicable={false}`
to render an inert "—" cell with an explanation tooltip.
-->
<script lang="ts">
  import { MatrixCellButton, type MatrixCellTone } from '$lib/ui/matrix';

  type State = 'allow' | 'deny' | 'neutral';

  let {
    override,
    inherited = 'neutral',
    effective,
    privilegedOnly = false,
    applicable = true,
    disabled = false,
    locked = false,
    allowBlocked = false,
    ceilingBlocked = false,
    decisionMode = 'tri-state',
    canDeny = true,
    updating = false,
    ariaLabel,
    title,
    onCycle
  }: {
    override: State;
    inherited?: State;
    /** The real result from the server. When set, it colors the cell. */
    effective?: State;
    /** True when the result is allowed only in privileged mode. */
    privilegedOnly?: boolean;
    applicable?: boolean;
    disabled?: boolean;
    /** Keep an inherited state visible while making the cell fully inert. */
    locked?: boolean;
    /** Skip the allow state when a delegation ceiling makes it invalid. */
    allowBlocked?: boolean;
    /** Marks a configured allow that is dormant under a delegation ceiling. */
    ceilingBlocked?: boolean;
    decisionMode?: 'tri-state' | 'binary';
    /** False for named roles, which can only allow. */
    canDeny?: boolean;
    updating?: boolean;
    ariaLabel: string;
    title?: string;
    onCycle: (next: State) => void;
  } = $props();

  function nextState(): State {
    if (decisionMode === 'binary') return visual === 'allow' ? 'neutral' : 'allow';
    if (!canDeny) return override === 'neutral' && !allowBlocked ? 'allow' : 'neutral';
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

  // The cell is colored by the server's effective result when known.
  // Otherwise by the *override* when present, else by the inherited baseline
  // (so a row's effective state is visible at a glance, matching the editor's
  // "permission name reflects effective state" rule).
  const visual = $derived(effective ?? (override !== 'neutral' ? override : inherited));
  const isOverride = $derived(override !== 'neutral');
  const interactionDisabled = $derived(
    disabled || locked || (decisionMode === 'binary' && allowBlocked && visual !== 'allow')
  );
  const displayLocked = $derived(locked || (allowBlocked && interactionDisabled));
  const icon = $derived.by(() => {
    if (privilegedOnly) return 'icon-[uil--shield-check]';
    if (visual === 'allow') return 'icon-[uil--check]';
    if (visual === 'deny' && decisionMode === 'tri-state') return 'icon-[uil--times]';
    return 'icon-[uil--minus]';
  });
  const tone = $derived.by<MatrixCellTone>(() => {
    if (ceilingBlocked) return 'warning';
    if (privilegedOnly) return 'neutral';
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
