<!--
@component

A small chip-shaped button. Works two ways:

- **Toggle**: caller drives a `pressed` prop and the chip renders an
  "active/selected" state when pressed. Use for Allow / Deny pairs in
  permission editors, on/off filter chips, etc.
- **Action**: leave `pressed` at its default (`false`) and the chip acts
  as a tinted icon/text button. Hover still tints toward `tone` so the
  intent is legible. The chip is the canonical secondary affordance —
  uniform shape, gradient, shadow, and ring vocabulary across actions
  and toggles.

```svelte
<ToggleChip
  pressed={state === 'allow'}
  tone="success"
  onclick={() => onSetState(perm, state === 'allow' ? 'neutral' : 'allow')}
>
  Allow
</ToggleChip>
```

For an action-style chip (no toggle), leave `pressed` unset and put an
iconify icon in the slot:

```svelte
<ToggleChip tone="danger" title="Delete" onclick={onDelete}>
  <span class="iconify uil--trash-alt"></span>
</ToggleChip>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'success' | 'danger' | 'warning' | 'primary' | 'neutral';

  let {
    children,
    pressed = false,
    tone = 'primary',
    square = false,
    disabled = false,
    onclick,
    title
  }: {
    children: Snippet;
    /** Whether the chip is in its active/selected state. */
    pressed?: boolean;
    /** Color used for the pressed gradient and the inactive hover tint. */
    tone?: Tone;
    /**
     * Render as a square icon-only chip (no horizontal padding, fixed
     * 28×28). Use for icon-only affordances so they don't gain bonus
     * width from `px-2.5`.
     */
    square?: boolean;
    disabled?: boolean;
    onclick?: (e: MouseEvent) => void;
    /** Native title attribute for hover hints. */
    title?: string;
  } = $props();

  // Pressed: subtle tone-tinted gradient + tone ring, so the chip reads
  // as "on" without needing drop shadows (we don't use shadows anywhere
  // else in the design language). Mirrors the gradient direction used
  // by MatrixCell and the btn-* utilities: top-light → bottom-saturated.
  const pressedClasses: Record<Tone, string> = {
    success:
      'bg-gradient-to-br from-success/25 to-success/45 text-success ring-1 ring-success/30 hover:from-success/35 hover:to-success/55',
    danger:
      'bg-gradient-to-br from-danger/25 to-danger/45 text-danger ring-1 ring-danger/30 hover:from-danger/35 hover:to-danger/55',
    warning:
      'bg-gradient-to-br from-warning/25 to-warning/45 text-warning ring-1 ring-warning/30 hover:from-warning/35 hover:to-warning/55',
    primary:
      'bg-gradient-to-br from-primary/25 to-primary/45 text-primary ring-1 ring-primary/30 hover:from-primary/35 hover:to-primary/55',
    neutral:
      'bg-gradient-to-br from-surface-200 to-surface-300 text-text ring-1 ring-text/10 hover:from-surface-300 hover:to-surface-300'
  };

  // Inactive: faint surface gradient + neutral ring. Hover tints toward
  // the tone to preview the "on" state.
  const inactiveClasses =
    'bg-gradient-to-br from-surface-100/80 to-surface-200/80 text-muted ring-1 ring-text/5';

  const inactiveHover: Record<Tone, string> = {
    success:
      'hover:from-success/10 hover:to-success/20 hover:text-success hover:ring-success/20',
    danger: 'hover:from-danger/10 hover:to-danger/20 hover:text-danger hover:ring-danger/20',
    warning:
      'hover:from-warning/10 hover:to-warning/20 hover:text-warning hover:ring-warning/20',
    primary:
      'hover:from-primary/10 hover:to-primary/20 hover:text-primary hover:ring-primary/20',
    neutral: 'hover:from-surface-200 hover:to-surface-300 hover:text-text hover:ring-text/10'
  };
</script>

<button
  type="button"
  class={[
    'inline-flex h-7 cursor-pointer items-center justify-center gap-1.5 rounded-md text-xs font-medium transition-all duration-150',
    square ? 'w-7' : 'min-w-7 px-2.5',
    pressed ? pressedClasses[tone] : [inactiveClasses, inactiveHover[tone]],
    disabled ? 'cursor-not-allowed opacity-60' : ''
  ]}
  {disabled}
  {title}
  aria-pressed={pressed}
  {onclick}
>
  {@render children()}
</button>
