<!--
@component

One public Chatto server profile. Server-supplied content stays inside the
card. Callers supply trusted badges and actions through explicit props.
-->
<script lang="ts" module>
  import type { PublicServerInfo } from '@chatto/client/api/server';
  import { serverHost } from '$lib/serverUrl';

  /** Public profile fields that the card renders. */
  export type ServerProfileCardProfile = Pick<
    PublicServerInfo,
    'name' | 'description' | 'iconUrl' | 'bannerUrl'
  >;
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import { m } from '$lib/i18n/messages';
  import { loadPublicServerImage, publicServerImageURL } from '$lib/publicServerImage';
  import { Pill, ScrollFader } from '$lib/ui';
  import { getGradientForName } from '$lib/utils/gradients';

  let {
    origin,
    imageOrigin = origin,
    profile,
    badge,
    footer,
    actions,
    onactivate,
    href,
    opensInNewTab = false,
    actionLabel,
    disabled = false,
    testId = 'server-profile-card',
    headingTag = 'h3'
  }: {
    origin: string;
    /**
     * Origin that may supply the logo and banner. It defaults to `origin`. A
     * registered server that hosts cached copies can supply them instead.
     */
    imageOrigin?: string;
    /** `undefined` means loading; `null` means that discovery failed. */
    profile?: ServerProfileCardProfile | null;
    badge?: string;
    /** Optional status content in a recessed, centred footer. */
    footer?: Snippet;
    actions?: Snippet;
    /** Makes the whole card perform the caller's existing open or join action. */
    onactivate?: () => void;
    /** Makes the whole card a link. This takes precedence over `onactivate`. */
    href?: string;
    /** Opens `href` in a separate browsing context without opener access. */
    opensInNewTab?: boolean;
    actionLabel?: string;
    disabled?: boolean;
    testId?: string;
    /** Heading level of the server name, one below the surrounding section. */
    headingTag?: 'h2' | 'h3' | 'h4';
  } = $props();

  let cardAction = $state<HTMLAnchorElement | HTMLButtonElement>();
  const interactive = $derived(!!href || !!onactivate);

  /** Keep text selectable and preserve link modifiers through the scroll viewport. */
  function activateFromDescription(event: MouseEvent) {
    if (window.getSelection()?.toString()) return;
    if (cardAction instanceof HTMLAnchorElement) {
      cardAction.dispatchEvent(
        new MouseEvent('click', {
          bubbles: true,
          cancelable: true,
          view: window,
          ctrlKey: event.ctrlKey,
          metaKey: event.metaKey,
          shiftKey: event.shiftKey,
          altKey: event.altKey
        })
      );
    } else {
      cardAction?.click();
    }
  }

  const hostname = $derived(serverHost(origin));
  const titleId = $props.id();
  const logoServer = $derived({
    name: profile?.name ?? hostname,
    logoUrl: profile?.iconUrl
  });
  const bannerURL = $derived(publicServerImageURL(imageOrigin, profile?.bannerUrl ?? null));
  let failedBannerURL = $state<string | null>(null);
  const accessibleActionLabel = $derived(actionLabel ?? logoServer.name);
</script>

<article
  class={[
    '@container/card relative isolate flex h-full min-w-0 flex-col overflow-hidden inset-frame text-start shadow-[var(--depth-shadow-sm)]',
    interactive && !disabled && 'shimmer-group hover-lift'
  ]}
  data-testid={testId}
  data-origin={origin}
>
  {#if href}
    <!-- eslint-disable svelte/no-navigation-without-resolve -- href is a caller-provided destination -->
    <a
      bind:this={cardAction}
      {href}
      target={opensInNewTab ? '_blank' : undefined}
      rel={opensInNewTab ? 'noopener noreferrer' : undefined}
      aria-label={accessibleActionLabel}
      aria-describedby={`${titleId} ${titleId}-host`}
      class="absolute inset-0 z-10 cursor-pointer rounded-[inherit] transition-colors hover:bg-text/5 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action"
      data-testid={`${testId}-action`}
      ><span class="sr-only">{actionLabel ?? logoServer.name}</span></a
    >
    <!-- eslint-enable svelte/no-navigation-without-resolve -->
  {:else if onactivate}
    <button
      bind:this={cardAction}
      type="button"
      onclick={onactivate}
      {disabled}
      aria-label={accessibleActionLabel}
      aria-describedby={`${titleId} ${titleId}-host`}
      class="absolute inset-0 z-10 cursor-pointer rounded-[inherit] transition-colors hover:bg-text/5 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action disabled:cursor-wait"
      data-testid={`${testId}-action`}
      ><span class="sr-only">{actionLabel ?? logoServer.name}</span></button
    >
  {/if}
  <div
    class={['shrink-0 overflow-hidden inset-content', interactive && !disabled && 'shimmer-hover']}
  >
    {#if bannerURL && failedBannerURL !== bannerURL}
      <img
        alt=""
        class="aspect-[1200/630] w-full shrink-0 object-cover"
        {@attach loadPublicServerImage(bannerURL)}
        onerror={() => (failedBannerURL = bannerURL)}
      />
    {:else}
      <!-- The name-seeded gradient matches the server's logo fallback. -->
      <div
        class="aspect-[1200/630] w-full shrink-0 opacity-40"
        class:bg-surface-emphasized={profile === undefined}
        style:background={profile === undefined ? undefined : getGradientForName(logoServer.name)}
        aria-hidden="true"
        data-banner-fallback
      ></div>
    {/if}
  </div>

  <div class="flex flex-1 flex-col gap-2 p-2.5 @min-[16rem]/card:gap-3 @min-[16rem]/card:p-4">
    <div
      class="flex min-w-0 shrink-0 flex-col items-start gap-2 @min-[16rem]/card:h-12 @min-[16rem]/card:flex-row @min-[16rem]/card:items-center @min-[16rem]/card:gap-3"
    >
      <div
        class="size-8 shrink-0 overflow-hidden rounded-lg border border-border bg-surface-emphasized @min-[16rem]/card:size-12 @min-[16rem]/card:rounded-xl"
      >
        <ServerLogo server={logoServer} publicImageOrigin={imageOrigin} fill />
      </div>

      <div class="w-full min-w-0 flex-1">
        <div class="flex min-w-0 items-center gap-2">
          <svelte:element
            this={headingTag}
            id={titleId}
            class="min-w-0 truncate font-semibold text-text-top"
          >
            <bdi dir="auto">{profile?.name ?? hostname}</bdi>
          </svelte:element>
          {#if badge}<Pill tone="success">{badge}</Pill>{/if}
        </div>
        <p
          id={`${titleId}-host`}
          class="truncate text-xs text-muted @min-[16rem]/card:text-base"
          dir="ltr"
        >
          {hostname}
        </p>
      </div>
    </div>

    <!-- A fixed reading area keeps every card's actions aligned. The shared
         scroll viewport makes long descriptions reachable by keyboard. -->
    <ScrollFader
      top
      bottom
      mask
      maskSize="1rem"
      fill={false}
      class={`relative z-20 h-16 shrink-0 @min-[16rem]/card:h-18${interactive ? ' cursor-pointer' : ''}`}
      onclick={interactive ? activateFromDescription : undefined}
      aria-labelledby={`${titleId} ${titleId}-host`}
      keyboardFocusable={!!profile?.description}
      data-testid="server-card-description"
    >
      {#if profile?.description}
        <p class="text-xs wrap-anywhere whitespace-pre-wrap text-muted @min-[16rem]/card:text-sm">
          <bdi dir="auto">{profile.description}</bdi>
        </p>
      {:else if profile === null}
        <p class="text-text">{m('add_server.directory.profile_unavailable')}</p>
      {/if}
    </ScrollFader>

    {#if actions}
      <div class="relative z-20 mt-auto">{@render actions()}</div>
    {/if}
  </div>
  {#if footer}
    <div
      class="pointer-events-none relative z-20 flex items-center justify-center inset-content px-2.5 py-2 text-center text-sm text-muted @min-[16rem]/card:px-4"
    >
      {@render footer()}
    </div>
  {/if}
</article>
