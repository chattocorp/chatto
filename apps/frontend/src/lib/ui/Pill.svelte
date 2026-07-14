<!--
@component

Small rounded badge for inline labels: scope tags ("Instance" / "Space"),
type tags ("System" / "Custom"), allow/deny override pills, level
indicators, and similar terse status decorations. Static — for
clickable toggleable variants use `<ToggleChip>`.

```svelte
<Pill tone="success">Allow from space</Pill>
<Pill tone="neutral">Space</Pill>
<Pill tone="muted">System</Pill>
<Pill tone="danger" dimmed>Inherited Allow (overridden)</Pill>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'success' | 'danger' | 'action' | 'neutral' | 'muted' | 'subtle' | 'server';

  let {
    children,
    tone = 'muted',
    dimmed = false,
    compact = false,
    title,
    class: className
  }: {
    children: Snippet;
    /** Color tone of the pill. */
    tone?: Tone;
    /**
     * Render dimmed with a strikethrough — useful for "this value is
     * overridden / no longer in effect" presentation.
     */
    dimmed?: boolean;
    /** Reduce horizontal padding for pills embedded in constrained chrome. */
    compact?: boolean;
    /** Native title attribute for hover hints. */
    title?: string;
    /**
     * Additional layout classes for the pill itself (e.g. `flex min-w-0
     * max-w-full` to make it shrink-friendly inside a constrained flex
     * parent so its content can truncate based on container width).
     */
    class?: string;
  } = $props();

  const toneClasses: Record<Tone, string> = {
    success: 'bg-success/10 text-success',
    danger: 'bg-danger/10 text-danger',
    action: 'bg-action/10 text-action',
    neutral: 'bg-neutral-action/10 text-neutral-action',
    muted: 'bg-surface-emphasized text-muted',
    subtle: 'bg-text/5 text-muted ring-1 ring-text/10 shadow-xs shadow-text/5',
    server: 'bg-server/10 text-server'
  };
</script>

<span
  {title}
  class={[
    'inline-block rounded py-0.5 text-xs font-medium',
    compact ? 'px-1' : 'px-2',
    toneClasses[tone],
    dimmed ? 'line-through' : '',
    className
  ]}
>
  {@render children()}
</span>
