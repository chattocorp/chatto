<!--
@component

Small rounded badge for inline labels: scope tags ("Instance" / "Space"),
type tags ("System" / "Custom"), allow/deny override pills, level
indicators, and similar terse status decorations. Static — for
clickable toggleable variants use `<ToggleChip>`.

```svelte
<Pill tone="success">Allow from space</Pill>
<Pill tone="primary">Space</Pill>
<Pill tone="muted">System</Pill>
<Pill tone="danger" dimmed>Inherited Allow (overridden)</Pill>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'success' | 'danger' | 'primary' | 'accent' | 'muted';

  let {
    children,
    tone = 'muted',
    dimmed = false,
    title
  }: {
    children: Snippet;
    /** Color tone of the pill. */
    tone?: Tone;
    /**
     * Render dimmed with a strikethrough — useful for "this value is
     * overridden / no longer in effect" presentation.
     */
    dimmed?: boolean;
    /** Native title attribute for hover hints. */
    title?: string;
  } = $props();

  const toneClasses: Record<Tone, string> = {
    success: 'bg-success/10 text-success',
    danger: 'bg-danger/10 text-danger',
    primary: 'bg-primary/10 text-primary',
    accent: 'bg-accent/10 text-accent',
    muted: 'bg-surface-200 text-muted'
  };
</script>

<span
  {title}
  class={[
    'inline-block rounded px-2 py-0.5 text-xs font-medium',
    toneClasses[tone],
    dimmed ? 'opacity-50 line-through' : ''
  ]}
>
  {@render children()}
</span>
