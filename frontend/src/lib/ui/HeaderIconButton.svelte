<!--
@component

The standard icon-only button used inside `PaneHeader` (and any other
header-style toolbar). Wraps a single iconify glyph in a button or
anchor with a fixed padded hit area, color tones, and hover behaviour,
so every pane header keeps the same visual rhythm.

Pass either `onclick` for a regular button or `href` for navigation —
the component renders the matching element and gets accessible name
from the required `label` prop.

```svelte
<HeaderIconButton icon="uil--bell" label="Follow thread" onclick={toggle} />
<HeaderIconButton icon="uil--bell" label="Unfollow thread" tone="active" onclick={toggle} />
<HeaderIconButton icon="uil--cog" label="Settings" href="/settings" />
<HeaderIconButton icon="uil--trash" label="Delete" tone="danger" onclick={destroy} />
```

For the "back" affordance to the left of a `PaneHeader` title, use
`PaneHeader`'s `backHref` / `onBack` props instead — those keep the
arrow aligned with the sidebar nav items below.
-->
<script lang="ts">
  type Tone = 'default' | 'active' | 'danger';

  let {
    icon,
    label,
    onclick,
    href,
    tone = 'default',
    disabled = false,
    title
  }: {
    /** Iconify utility class (e.g. `'uil--bell'`). */
    icon: string;
    /** Accessible label. Also used as the default `title` (hover hint). */
    label: string;
    /** Click handler for the button variant. Ignored when `href` is set. */
    onclick?: (event: MouseEvent) => void;
    /** Render as an anchor link instead of a button. */
    href?: string;
    /**
     * Visual tone:
     * - `default` (muted text → text on hover)
     * - `active` (selected background — for toggled-on states like "following")
     * - `danger` (red tint with red hover)
     */
    tone?: Tone;
    /** Disabled state — only applies to the button variant. */
    disabled?: boolean;
    /** Override the default hover tooltip (defaults to `label`). */
    title?: string;
  } = $props();

  const toneClasses: Record<Tone, string> = {
    default: 'text-muted',
    active: 'pane-header-icon-button-active',
    danger: 'text-danger'
  };

  const buttonClass = $derived([
    'group/pane-header-icon-button pane-header-icon-button',
    toneClasses[tone]
  ]);
  const glyphClass = $derived(['pane-header-icon-glyph', icon]);
</script>

{#if href}
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href is a prop; callers pass already-resolved paths -->
  <a {href} class={buttonClass} title={title ?? label} aria-label={label}>
    <span class={glyphClass} aria-hidden="true"></span>
  </a>
{:else}
  <button
    type="button"
    class={buttonClass}
    {disabled}
    {onclick}
    title={title ?? label}
    aria-label={label}
  >
    <span class={glyphClass} aria-hidden="true"></span>
  </button>
{/if}
