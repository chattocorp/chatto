<!--
@component

A small notice/hint box for inline contextual information at the top of a page
or above a form. Use to explain a non-obvious concept ("room overrides take
precedence over space settings"), point at a related action, or warn about a
caveat — anything that isn't a hard error.

For hard errors use the danger tone or a dedicated error component
(`FormError`). For toast-style transient feedback use `toast.*` from
`$lib/ui/toast`.

The `tone` prop tints the background, border, and text. The default tone is
`info`, which uses the muted surface color so it sits quietly at the top of
a page without competing with primary content.

```svelte
<Hint>Room overrides take precedence over space-level role configuration.</Hint>

<Hint tone="warning" icon="uil--exclamation-triangle">
  Changes here apply immediately to all members.
</Hint>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'info' | 'warning' | 'success' | 'danger';

  let {
    children,
    tone = 'info',
    icon
  }: {
    children: Snippet;
    /** Tint of the hint. Defaults to a muted info style. */
    tone?: Tone;
    /**
     * Iconify class name (e.g. `'uil--info-circle'`). Defaults to a sensible
     * icon per tone. Pass `null` to suppress the icon.
     */
    icon?: string | null;
  } = $props();

  const toneStyles: Record<Tone, string> = {
    info: 'border-border bg-surface text-muted',
    warning: 'border-warning/30 bg-warning/10 text-warning',
    success: 'border-success/30 bg-success/10 text-success',
    danger: 'border-danger/30 bg-danger/10 text-danger'
  };

  const defaultIcons: Record<Tone, string> = {
    info: 'uil--info-circle',
    warning: 'uil--exclamation-triangle',
    success: 'uil--check-circle',
    danger: 'uil--times-circle'
  };

  const resolvedIcon = $derived(icon === null ? null : (icon ?? defaultIcons[tone]));
</script>

<div class={['flex items-start gap-3 rounded-lg border p-4', toneStyles[tone]]}>
  {#if resolvedIcon}
    <span class={['mt-0.5 iconify shrink-0 text-lg', resolvedIcon]}></span>
  {/if}
  <div class="min-w-0 flex-1">
    {@render children()}
  </div>
</div>
