<!--
@component

One public Chatto server profile. Server-supplied content stays inside the
card. Callers supply trusted badges and actions through explicit props.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { PublicServerInfo } from '$lib/api-client/server';
  import { m } from '$lib/i18n/messages';
  import { Pill, SkeletonImg } from '$lib/ui';

  let {
    origin,
    profile,
    badge,
    actions,
    testId = 'server-profile-card'
  }: {
    origin: string;
    /** `undefined` means loading; `null` means that discovery failed. */
    profile?: PublicServerInfo | null;
    badge?: string;
    actions?: Snippet;
    testId?: string;
  } = $props();

  const hostname = $derived.by(() => {
    try {
      return new URL(origin).host;
    } catch {
      return origin;
    }
  });
</script>

<article
  class="flex min-h-64 flex-col overflow-hidden rounded-xl border border-border bg-surface"
  data-testid={testId}
  data-origin={origin}
>
  {#if profile?.bannerUrl}
    <SkeletonImg src={profile.bannerUrl} alt="" class="h-32 w-full object-cover" />
  {:else}
    <div class="h-32 shrink-0"></div>
  {/if}

  <div class="flex flex-1 flex-col gap-4 p-4">
    <div class="flex min-w-0 items-start gap-3">
      <div
        class="-mt-10 flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-lg border-2 border-border bg-surface-emphasized"
      >
        {#if profile?.iconUrl}
          <SkeletonImg src={profile.iconUrl} alt="" class="h-full w-full object-cover" />
        {:else}
          <span class="iconify icon-[uil--globe] text-2xl text-muted" aria-hidden="true"></span>
        {/if}
      </div>

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

    {#if actions}
      {@render actions()}
    {/if}
  </div>
</article>
