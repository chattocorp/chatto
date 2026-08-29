<!--
@component

One public server in the Server Directory. Server-supplied profile content is
kept inside the card; action labels remain trusted, static client copy.
-->
<script lang="ts">
  import type { PublicServerInfo } from '$lib/api-client/server';
  import { m } from '$lib/i18n/messages';
  import { Pill, SkeletonImg } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let {
    origin,
    profile,
    joined = false,
    actionLabel,
    actionLoading = false,
    actionDisabled = false,
    onaction
  }: {
    origin: string;
    profile: PublicServerInfo | null;
    joined?: boolean;
    actionLabel?: string;
    actionLoading?: boolean;
    actionDisabled?: boolean;
    onaction?: () => void;
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
  data-testid="server-directory-entry"
  data-origin={origin}
>
  {#if profile?.bannerUrl}
    <SkeletonImg src={profile.bannerUrl} alt="" class="h-28 w-full object-cover" />
  {:else}
    <div class="h-16 shrink-0 bg-surface-emphasized"></div>
  {/if}

  <div class="flex flex-1 flex-col gap-4 p-4">
    <div class="flex min-w-0 items-start gap-3">
      <div
        class={[
          'flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border bg-surface-emphasized',
          profile?.bannerUrl && '-mt-10'
        ]}
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
          {#if joined}<Pill tone="success">{m('add_server.directory.joined')}</Pill>{/if}
        </div>
        <p class="truncate text-sm text-muted" dir="ltr">{hostname}</p>
      </div>
    </div>

    <div class="flex-1">
      {#if profile?.description}
        <p class="line-clamp-3 text-sm text-muted"><bdi dir="auto">{profile.description}</bdi></p>
      {:else if !profile}
        <p class="text-sm text-muted">{m('add_server.directory.profile_unavailable')}</p>
      {/if}
    </div>

    {#if actionLabel && onaction}
      <Button
        variant={joined ? 'secondary' : 'action'}
        fullWidth
        loading={actionLoading}
        disabled={actionDisabled}
        onclick={onaction}
      >
        {actionLabel}
      </Button>
    {/if}
  </div>
</article>
