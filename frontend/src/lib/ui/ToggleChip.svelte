<!--
@component

A small rounded "pill" button with a pressed state and a tone color.
Use for toggleable status indicators where the chip is the toggle: Allow
/ Deny pairs in permission editors, on/off filter chips, etc.

Distinct from `<Button>`: smaller padding, pill shape, opaque background
in both states, and a binary `pressed` prop that the caller controls.
The chip itself doesn't manage its own state — flip `pressed` from the
parent on click.

```svelte
<ToggleChip
  pressed={state === 'allow'}
  tone="success"
  onclick={() => onSetState(perm, state === 'allow' ? 'neutral' : 'allow')}
>
  Allow
</ToggleChip>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'success' | 'danger' | 'primary' | 'neutral';

  let {
    children,
    pressed = false,
    tone = 'primary',
    disabled = false,
    onclick,
    title
  }: {
    children: Snippet;
    /** Whether the chip is in its active/selected state. */
    pressed?: boolean;
    /** Color used when the chip is pressed. Inactive chips share a neutral look. */
    tone?: Tone;
    disabled?: boolean;
    onclick?: (e: MouseEvent) => void;
    /** Native title attribute for hover hints. */
    title?: string;
  } = $props();

  const pressedClasses: Record<Tone, string> = {
    success: 'bg-success/25 text-success',
    danger: 'bg-danger/25 text-danger',
    primary: 'bg-primary/25 text-primary',
    neutral: 'bg-surface-300 text-text'
  };

  const inactiveHover: Record<Tone, string> = {
    success: 'hover:bg-success/15 hover:text-success',
    danger: 'hover:bg-danger/15 hover:text-danger',
    primary: 'hover:bg-primary/15 hover:text-primary',
    neutral: 'hover:bg-surface-300 hover:text-text'
  };
</script>

<button
  type="button"
  class={[
    'cursor-pointer rounded-full px-3 py-1 text-xs font-medium transition-colors',
    pressed ? pressedClasses[tone] : ['bg-surface text-muted', inactiveHover[tone]],
    disabled ? 'cursor-not-allowed opacity-60' : ''
  ]}
  {disabled}
  {title}
  aria-pressed={pressed}
  {onclick}
>
  {@render children()}
</button>
