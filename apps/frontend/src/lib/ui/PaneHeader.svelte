<!--
@component

The standard pane-level header used at the top of every secondary
panel (admin pages, settings, room view, thread pane, …). Provides a
consistent layout of:

  [back affordance]  Title  [subtitle]                 [actions...]

Design language:

  - Left padding is `pl-2` when a back affordance is shown, `pl-4`
    otherwise. The reduced left inset lines the back arrow up with the
    sidebar-nav items rendered below the header.
  - Header icons use a fixed padded hit area so optional backgrounds do
    not change pane header height.
  - Right-side action icons are `<HeaderIconButton>` instances passed
    via the `actions` snippet. They use the same fixed hit area and
    glyph size as other pane-header icons.

Use `backHref` for navigation-style "back to parent route" affordances
(renders an anchor) or `onBack` for callback-style "close this slideover
/ overlay" affordances (renders a button). Exactly one of the two should
be set; if both are passed the button wins (it's the more deliberate
choice).
-->
<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- backHref is a prop; callers pass already-resolved paths or non-route hrefs */
  import type { Snippet } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import PaneHeaderSkeleton from './PaneHeaderSkeleton.svelte';

  let {
    title,
    subtitle,
    loading = false,
    skeletonButtons = 3,
    afterTitle,
    actions,
    backHref,
    onBack,
    backLabel = m['ui.pane_header.back'](),
    // Deprecated: showMobileNav is no longer used since hamburger menu is always visible
    showMobileNav: _showMobileNav = false
  }: {
    title: string;
    subtitle?: string;
    loading?: boolean;
    skeletonButtons?: number;
    afterTitle?: Snippet;
    actions?: Snippet;
    /**
     * Render a left-arrow back link before the title. Use for detail
     * pages so callers don't have to stuff a full secondary <Button>
     * into `actions` (which exploded the header height).
     */
    backHref?: string;
    /**
     * Render a left-arrow back button before the title. Use for
     * slideover panels and overlays whose "back" doesn't navigate.
     * Takes precedence over `backHref` when both are provided.
     */
    onBack?: (event: MouseEvent) => void;
    /** Title attribute / aria-label for the back affordance. */
    backLabel?: string;
    showMobileNav?: boolean;
  } = $props();

  const hasBack = $derived(onBack !== undefined || backHref !== undefined);
</script>

<div
  class={[
    'flex h-14 shrink-0 items-center justify-between border-b border-border pr-4',
    hasBack ? 'pl-2' : 'pl-4'
  ]}
>
  <div class={['flex min-w-0 flex-1 items-center', hasBack ? 'gap-2' : 'gap-3']}>
    {#if onBack}
      <button
        type="button"
        class="group/pane-header-icon-button pane-header-icon-button"
        onclick={onBack}
        title={backLabel}
        aria-label={backLabel}
      >
        <span class="pane-header-icon-glyph uil--arrow-left" aria-hidden="true"></span>
      </button>
    {:else if backHref}
      <a
        href={backHref}
        class="group/pane-header-icon-button pane-header-icon-button"
        title={backLabel}
        aria-label={backLabel}
      >
        <span class="pane-header-icon-glyph uil--arrow-left" aria-hidden="true"></span>
      </a>
    {/if}
    <div class="flex min-w-0 flex-1 flex-col gap-1 md:flex-row md:items-baseline md:gap-3">
      {#if loading}
        <PaneHeaderSkeleton buttons={skeletonButtons} />
      {:else}
        <div class="flex min-w-0 items-baseline gap-3">
          <h1 class="truncate font-black">{title}</h1>
          {#if afterTitle}
            <div class="shrink-0">
              {@render afterTitle()}
            </div>
          {/if}
        </div>
      {/if}
      {#if subtitle}
        <span class="hidden truncate text-sm text-muted md:inline">{subtitle}</span>
      {/if}
    </div>
  </div>
  {#if actions}
    <div class="flex items-center gap-2">
      {@render actions()}
    </div>
  {/if}
</div>
