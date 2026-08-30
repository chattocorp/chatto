<!--
@component

One public Chatto server profile. Server-supplied content stays inside the
card. Callers supply trusted badges and actions through explicit props.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { PublicServerInfo } from '$lib/api-client/server';
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import { m } from '$lib/i18n/messages';
  import { Pill, SkeletonImg } from '$lib/ui';

  let {
    origin,
    profile,
    badge,
    details,
    actions,
    onIconClick,
    iconHref,
    iconOpensInNewTab = false,
    iconActionLabel,
    iconActionDisabled = false,
    testId = 'server-profile-card'
  }: {
    origin: string;
    /** `undefined` means loading; `null` means that discovery failed. */
    profile?: PublicServerInfo | null;
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
  const accessibleIconActionLabel = $derived(
    iconActionLabel ? `${iconActionLabel}: ${logoServer.name}` : logoServer.name
  );
</script>

<article
  class="flex min-h-64 flex-col overflow-hidden rounded-xl border border-border bg-surface"
  data-testid={testId}
  data-origin={origin}
>
  {#if profile?.bannerUrl}
    <SkeletonImg src={profile.bannerUrl} alt="" class="h-32 w-full object-cover" />
  {:else}
    <div
      class="h-32 shrink-0 bg-gradient-to-br from-surface-emphasized/80 via-surface-emphasized/45 to-surface"
    ></div>
  {/if}

  <div class="flex flex-1 flex-col gap-4 p-4">
    <div class="flex min-w-0 items-start gap-3">
      {#if iconHref}
        <!-- eslint-disable svelte/no-navigation-without-resolve -- iconHref is a caller-provided external URL -->
        <a
          href={iconHref}
          target={iconOpensInNewTab ? '_blank' : undefined}
          rel={iconOpensInNewTab ? 'noopener noreferrer' : undefined}
          class="-mt-10 h-14 w-14 shrink-0 cursor-pointer overflow-hidden rounded-xl border-2 border-border bg-surface-emphasized transition-transform focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action active:scale-[0.96]"
          aria-label={accessibleIconActionLabel}
          title={accessibleIconActionLabel}
          data-testid={`${testId}-icon-action`}
        >
          <ServerLogo server={logoServer} fill />
        </a>
        <!-- eslint-enable svelte/no-navigation-without-resolve -->
      {:else if onIconClick}
        <button
          type="button"
          class="-mt-10 h-14 w-14 shrink-0 cursor-pointer overflow-hidden rounded-xl border-2 border-border bg-surface-emphasized transition-transform focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action active:scale-[0.96] disabled:pointer-events-none disabled:cursor-not-allowed"
          aria-label={accessibleIconActionLabel}
          title={accessibleIconActionLabel}
          disabled={iconActionDisabled}
          onclick={onIconClick}
          data-testid={`${testId}-icon-action`}
        >
          <ServerLogo server={logoServer} fill />
        </button>
      {:else}
        <div
          class="-mt-10 h-14 w-14 shrink-0 overflow-hidden rounded-xl border-2 border-border bg-surface-emphasized"
        >
          <ServerLogo server={logoServer} fill />
        </div>
      {/if}

      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 flex-wrap items-center gap-2">
          <h3 class="min-w-0 truncate font-semibold text-text-top">
            <bdi dir="auto">{profile?.name ?? hostname}</bdi>
          </h3>
          {#if badge}<Pill tone="success">{badge}</Pill>{/if}
        </div>
        <p class="truncate text-sm text-muted" dir="ltr">{hostname}</p>
      </div>
    </div>

    <div class="flex-1">
      {#if profile?.description}
        <p class="line-clamp-3 text-sm text-muted"><bdi dir="auto">{profile.description}</bdi></p>
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
