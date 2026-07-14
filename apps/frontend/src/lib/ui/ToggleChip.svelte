<!--
@component

A small chip-shaped button. Works two ways:

- **Toggle**: caller drives a `pressed` prop and the chip renders an
  "active/selected" state when pressed. Use for Allow / Deny pairs in
  permission editors, on/off filter chips, etc.
- **Action**: leave `pressed` at its default (`false`) and the chip acts
  as a tinted icon/text button. Hover still tints toward `tone` so the
  intent is legible. The chip is the canonical compact secondary affordance,
  with one consistent shape and state vocabulary across actions and toggles.

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

  type Tone = 'success' | 'danger' | 'warning' | 'action' | 'neutral';

  let {
    children,
    pressed = false,
    tone = 'action',
    square = false,
    disabled = false,
    onclick,
    title
  }: {
    children: Snippet;
    /** Whether the chip is in its active/selected state. */
    pressed?: boolean;
    /** Color used for the pressed state and inactive hover tint. */
    tone?: Tone;
    /**
     * Render as a square icon-only chip (no horizontal padding, fixed
     * 40×40). Use for icon-only affordances so they don't gain bonus
     * width from `px-2.5`.
     */
    square?: boolean;
    disabled?: boolean;
    onclick?: (e: MouseEvent) => void;
    /** Native title attribute for hover hints. */
    title?: string;
  } = $props();

  const pressedClasses: Record<Tone, string> = {
    success: 'border-success/30 bg-success/12 text-success hover:bg-success/18',
    danger: 'border-danger/30 bg-danger/12 text-danger hover:bg-danger/18',
    warning: 'border-warning/30 bg-warning/12 text-warning hover:bg-warning/18',
    action: 'border-action/30 bg-action/10 text-action hover:bg-action/15',
    neutral: 'border-border bg-surface-strong text-text hover:bg-surface-selected'
  };

  const inactiveClasses = 'border-border bg-surface text-muted';

  const inactiveHover: Record<Tone, string> = {
    success: 'hover:border-success/25 hover:bg-success/8 hover:text-success',
    danger: 'hover:border-danger/25 hover:bg-danger/8 hover:text-danger',
    warning: 'hover:border-warning/25 hover:bg-warning/8 hover:text-warning',
    action: 'hover:border-action/25 hover:bg-action/8 hover:text-action',
    neutral: 'hover:bg-surface-emphasized hover:text-text'
  };
</script>

<button
  type="button"
  class={[
    'inline-flex min-h-10 cursor-pointer items-center justify-center gap-1.5 rounded-md border text-xs font-medium transition-[background-color,border-color,color,scale] duration-150 active:scale-[0.97]',
    square ? 'w-10' : 'min-w-10 px-2.5',
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
