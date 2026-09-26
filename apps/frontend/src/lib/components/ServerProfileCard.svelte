<!--
@component

One public Chatto server profile. Server-supplied content stays inside the
card. Callers supply trusted badges and actions through explicit props.

Place cards inside an `@container/server-cards` element. When that container
is narrower than 40rem, the card uses its compact layout: a thin banner strip,
a smaller logo, and tighter spacing.
-->
<script lang="ts" module>
  import type { PublicServerInfo } from '$lib/api-client/server';

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
  import { Pill } from '$lib/ui';
  import { getGradientForName } from '$lib/utils/gradients';

  let {
    origin,
    imageOrigin = origin,
    profile,
    badge,
    details,
    actions,
    onIconClick,
    iconHref,
    iconOpensInNewTab = false,
    iconActionLabel,
    iconActionDisabled = false,
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
    /** Optional caller-owned content between the public profile and actions. */
    details?: Snippet;
    actions?: Snippet;
    /** Makes the server icon perform the caller's existing open or join action. */
    onIconClick?: () => void;
    /** Makes the server icon a link. This takes precedence over `onIconClick`. */
    iconHref?: string;
    /** Opens `iconHref` in a separate browsing context without opener access. */
    iconOpensInNewTab?: boolean;
    iconActionLabel?: string;
    iconActionDisabled?: boolean;
    testId?: string;
    /** Heading level of the server name, one below the surrounding section. */
    headingTag?: 'h3' | 'h4';
  } = $props();

  const hostname = $derived.by(() => {
    try {
      return new URL(origin).host;
    } catch {
      return origin;
    }
  });
  const logoServer = $derived({
    name: profile?.name ?? hostname,
    logoUrl: profile?.iconUrl
  });
  const bannerURL = $derived(publicServerImageURL(imageOrigin, profile?.bannerUrl ?? null));
  let failedBannerURL = $state<string | null>(null);
  const accessibleIconActionLabel = $derived(
    iconActionLabel ? `${iconActionLabel}: ${logoServer.name}` : logoServer.name
  );
</script>

<article
  class="flex h-full flex-col overflow-hidden rounded-xl border border-border bg-surface"
  data-testid={testId}
  data-origin={origin}
>
  {#if bannerURL && failedBannerURL !== bannerURL}
    <img
      alt=""
      class="h-24 w-full object-cover @max-[40rem]/server-cards:h-10"
      {@attach loadPublicServerImage(bannerURL)}
      onerror={() => (failedBannerURL = bannerURL)}
    />
  {:else}
    <!-- The name-seeded gradient matches the server's logo fallback. -->
    <div
      class="h-24 shrink-0 opacity-40 @max-[40rem]/server-cards:h-10"
      class:bg-surface-emphasized={profile === undefined}
      style:background={profile === undefined ? undefined : getGradientForName(logoServer.name)}
      aria-hidden="true"
      data-banner-fallback
    ></div>
  {/if}

  <div
    class="flex flex-1 flex-col gap-3 p-4 @max-[40rem]/server-cards:gap-2 @max-[40rem]/server-cards:p-3"
  >
    <div class="flex min-w-0 items-start gap-3">
      {#if iconHref}
        <!-- eslint-disable svelte/no-navigation-without-resolve -- iconHref is a caller-provided external URL -->
        <a
          href={iconHref}
          target={iconOpensInNewTab ? '_blank' : undefined}
          rel={iconOpensInNewTab ? 'noopener noreferrer' : undefined}
          class="-mt-10 h-14 w-14 shrink-0 cursor-pointer overflow-hidden rounded-xl border-2 border-border bg-surface-emphasized transition-transform focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action active:scale-[0.96] @max-[40rem]/server-cards:-mt-6 @max-[40rem]/server-cards:h-10 @max-[40rem]/server-cards:w-10 @max-[40rem]/server-cards:rounded-lg"
          aria-label={accessibleIconActionLabel}
          title={accessibleIconActionLabel}
          data-testid={`${testId}-icon-action`}
        >
          <ServerLogo server={logoServer} publicImageOrigin={imageOrigin} fill />
        </a>
        <!-- eslint-enable svelte/no-navigation-without-resolve -->
      {:else if onIconClick}
        <button
          type="button"
          class="-mt-10 h-14 w-14 shrink-0 cursor-pointer overflow-hidden rounded-xl border-2 border-border bg-surface-emphasized transition-transform focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action active:scale-[0.96] disabled:pointer-events-none disabled:cursor-not-allowed @max-[40rem]/server-cards:-mt-6 @max-[40rem]/server-cards:h-10 @max-[40rem]/server-cards:w-10 @max-[40rem]/server-cards:rounded-lg"
          aria-label={accessibleIconActionLabel}
          title={accessibleIconActionLabel}
          disabled={iconActionDisabled}
          onclick={onIconClick}
          data-testid={`${testId}-icon-action`}
        >
          <ServerLogo server={logoServer} publicImageOrigin={imageOrigin} fill />
        </button>
      {:else}
        <div
          class="-mt-10 h-14 w-14 shrink-0 overflow-hidden rounded-xl border-2 border-border bg-surface-emphasized @max-[40rem]/server-cards:-mt-6 @max-[40rem]/server-cards:h-10 @max-[40rem]/server-cards:w-10 @max-[40rem]/server-cards:rounded-lg"
        >
          <ServerLogo server={logoServer} publicImageOrigin={imageOrigin} fill />
        </div>
      {/if}

      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 flex-wrap items-center gap-2">
          <svelte:element this={headingTag} class="min-w-0 truncate font-semibold text-text-top">
            <bdi dir="auto">{profile?.name ?? hostname}</bdi>
          </svelte:element>
          {#if badge}<Pill tone="success">{badge}</Pill>{/if}
        </div>
        <p class="truncate text-sm text-muted" dir="ltr">{hostname}</p>
      </div>
    </div>

    <div class="flex-1">
      {#if profile?.description}
        <p class="line-clamp-2 text-sm text-muted"><bdi dir="auto">{profile.description}</bdi></p>
      {:else if profile === null}
        <p class="text-sm text-muted">{m('add_server.directory.profile_unavailable')}</p>
      {/if}
    </div>

    {#if details}
      {@render details()}
    {/if}

    {#if actions}
      {@render actions()}
    {/if}
  </div>
</article>
