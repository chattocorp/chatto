<!--
@component

The standard pane-level header used at the top of every secondary
panel (admin pages, settings, room view, thread pane, …). Provides a
consistent layout of:

  [back affordance]  Title  [subtitle]                 [actions...]

Design language:

  - Start padding is `ps-2` when a back affordance is shown, `ps-4`
    otherwise. The reduced start inset lines the back arrow up with the
    sidebar-nav items rendered below the header.
  - Header icons use a fixed padded hit area so optional backgrounds do
    not change pane header height.
  - End-side action icons are `<HeaderIconButton>` instances passed
    via the `actions` snippet. They use the same fixed hit area and
    glyph size as other pane-header icons.

Use `backHref` for navigation-style "back to parent route" affordances
(renders an anchor) or `onBack` for callback-style "close this slideover
/ overlay" affordances (renders a button). Exactly one of the two should
be set; if both are passed the button wins (it's the more deliberate
choice).

Set `collapseActions` to put actions behind a three-dot button below 32 rem
of pane width. `collapsedActions` can keep important actions visible in that
state. Expansion stays inside the header and lets the title truncate.

Set `hideOnKeyboard` to remove the header from the mobile layout while the
shared viewport detector reports an open software keyboard.
-->
<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- backHref is a prop; callers pass already-resolved paths or non-route hrefs */
  import type { Snippet } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import PaneHeaderSkeleton from './PaneHeaderSkeleton.svelte';

  let {
    title,
    subtitle,
    loading = false,
    skeletonButtons = 3,
    afterTitle,
    actions,
    collapseActions = false,
    hideOnKeyboard = false,
    collapsedActions,
    actionsLabel = m('ui.pane_header.actions'),
    backHref,
    onBack,
    backLabel = m('ui.pane_header.back'),
    // Deprecated: showMobileNav is no longer used since hamburger menu is always visible
    showMobileNav: _showMobileNav = false
  }: {
    title: string;
    subtitle?: string;
    loading?: boolean;
    skeletonButtons?: number;
    afterTitle?: Snippet;
    actions?: Snippet;
    /** Collapse actions below 32 rem of pane width. Expand them beside the title. */
    collapseActions?: boolean;
    /** Hide below the md breakpoint while the software keyboard is detected. */
    hideOnKeyboard?: boolean;
    /** Important actions to show instead of the full list while collapsed. */
    collapsedActions?: Snippet;
    /** Accessible label for the action disclosure button. */
    actionsLabel?: string;
    /**
     * Render a direction-aware back link before the title. Use for detail
     * pages so callers don't have to stuff a full secondary <Button>
     * into `actions` (which exploded the header height).
     */
    backHref?: string;
    /**
     * Render a direction-aware back button before the title. Use for
     * slideover panels and overlays whose "back" doesn't navigate.
     * Takes precedence over `backHref` when both are provided.
     */
    onBack?: (event: MouseEvent) => void;
    /** Title attribute / aria-label for the back affordance. */
    backLabel?: string;
    showMobileNav?: boolean;
  } = $props();

  const hasBack = $derived(onBack !== undefined || backHref !== undefined);
  const actionsId = $props.id();
  let actionsExpanded = $state(false);
  let actionsButton = $state<HTMLButtonElement>();

  function handleEscape(event: KeyboardEvent) {
    if (
      event.key !== 'Escape' ||
      event.defaultPrevented ||
      !actionsExpanded ||
      !actionsButton?.getClientRects().length ||
      actionsButton.closest('[inert]')
    ) {
      return;
    }

    event.preventDefault();
    actionsExpanded = false;
    actionsButton.focus();
  }
</script>

<svelte:window onkeydown={handleEscape} />

<div
  class={[
    'shrink-0',
    collapseActions && '@container/pane-header',
    hideOnKeyboard && 'keyboard-hide-mobile'
  ]}
>
  <div
    data-page-reveal
    class={[
      'flex h-14 shrink-0 items-center justify-between border-b border-border pe-2',
      hasBack ? 'ps-2' : 'ps-4'
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
          <span
            class="icon-[uil--arrow-left] pane-header-icon-glyph text-xl rtl:-scale-x-100"
            aria-hidden="true"
          ></span>
        </button>
      {:else if backHref}
        <a
          href={backHref}
          class="group/pane-header-icon-button pane-header-icon-button"
          title={backLabel}
          aria-label={backLabel}
        >
          <span
            class="icon-[uil--arrow-left] pane-header-icon-glyph text-xl rtl:-scale-x-100"
            aria-hidden="true"
          ></span>
        </a>
      {/if}
      <div class="flex min-w-0 flex-1 flex-col gap-1 md:flex-row md:items-baseline md:gap-3">
        {#if loading}
          <PaneHeaderSkeleton buttons={skeletonButtons} />
        {:else}
          <div class="flex min-w-0 items-baseline gap-3">
            <h1 class="truncate font-black"><bdi>{title}</bdi></h1>
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
      <div class="flex shrink-0 items-center">
        <div
          id={actionsId}
          class={[
            'items-center',
            collapseActions
              ? [
                  '@min-[32rem]/pane-header:flex @min-[32rem]/pane-header:gap-2',
                  actionsExpanded ? 'flex' : 'hidden'
                ]
              : 'flex gap-2'
          ]}
        >
          {@render actions()}
        </div>
        {#if collapseActions}
          {#if !actionsExpanded && collapsedActions}
            <div class="flex items-center @min-[32rem]/pane-header:hidden">
              {@render collapsedActions()}
            </div>
          {/if}
          <button
            bind:this={actionsButton}
            type="button"
            class="group/pane-header-icon-button pane-header-icon-button @min-[32rem]/pane-header:hidden"
            aria-label={actionsLabel}
            aria-expanded={actionsExpanded}
            aria-controls={actionsId}
            onclick={() => (actionsExpanded = !actionsExpanded)}
          >
            <span class="icon-[uil--ellipsis-v] pane-header-icon-glyph" aria-hidden="true"></span>
          </button>
        {/if}
      </div>
    {/if}
  </div>
</div>
