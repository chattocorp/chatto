<!--
@component

The standard icon-only button used inside `PaneHeader` (and any other
header-style toolbar). Wraps a single iconify glyph in a button or
anchor with a fixed padded hit area, color tones, and hover behaviour,
so every pane header keeps the same visual rhythm.

Pass either `onclick` for a regular button or `href` for navigation —
the component renders the matching element and gets accessible name
from the required `label` prop.

Use `tone="active"` only to mark the panel that the button currently
shows. For an on/off setting, change the icon and label; do not add a
selected background.

```svelte
<HeaderIconButton icon="icon-[uil--users-alt]" label="Show members" onclick={toggle} />
<HeaderIconButton icon="icon-[uil--users-alt]" label="Hide members" tone="active" onclick={toggle} />
<HeaderIconButton icon="icon-[uil--bell]" label="Unfollow thread" onclick={toggle} />
<HeaderIconButton icon="icon-[uil--cog]" label="Settings" href="/settings" />
<HeaderIconButton icon="icon-[uil--trash]" label="Delete" tone="danger" onclick={destroy} />
```

For the "back" affordance to the left of a `PaneHeader` title, use
`PaneHeader`'s `backHref` / `onBack` props instead — those keep the
arrow aligned with the sidebar nav items below.
-->
<script lang="ts">
  type Tone = 'default' | 'active' | 'danger';
  type IconSize = 'sm' | 'md' | 'lg';

  let {
    icon,
    label,
    onclick,
    href,
    tone = 'default',
    iconSize = 'md',
    disabled = false,
    mirrorInRtl = false,
    title
  }: {
    /** Iconify utility class (e.g. `'icon-[uil--bell]'`). */
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
     * - `active` (action tint — marks the panel that this button currently shows)
     * - `danger` (red tint with red hover)
     */
    tone?: Tone;
    /** Fine-tune optical icon size when glyphs from different icon sets read unevenly. */
    iconSize?: IconSize;
    /** Disabled state — only applies to the button variant. */
    disabled?: boolean;
    /** Mirror a directional glyph when the document uses right-to-left layout. */
    mirrorInRtl?: boolean;
    /** Override the default hover tooltip (defaults to `label`). */
    title?: string;
  } = $props();

  const toneClasses: Record<Tone, string> = {
    default: 'text-muted',
    active: 'pane-header-icon-button-active',
    danger: 'text-danger'
  };
  const iconSizeClasses: Record<IconSize, string> = {
    sm: 'text-sm',
    md: 'text-base',
    lg: 'text-lg'
  };

  const buttonClass = $derived([
    'pane-header-icon-button',
    toneClasses[tone]
  ]);
  const glyphClass = $derived([
    'pane-header-icon-glyph',
    iconSizeClasses[iconSize],
    icon,
    mirrorInRtl && 'rtl:-scale-x-100'
  ]);
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
