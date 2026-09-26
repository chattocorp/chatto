<!--
@component

Displays a preview card for a Chatto message link (e.g. pasted in the composer
or embedded in a posted message). The message is fetched through the appropriate
instance's Connect timeline API; if it can't be loaded (not found, no permission,
unknown instance) the component renders nothing. A reconnect keeps the loaded
preview on screen; a snapshot after a long disconnect reloads it in place.

**Props:**
- `link` — Parsed MessageLink from `$lib/messageLinks`.
- `onDismiss` — Callback when user dismisses the preview (composer mode).
- `showDismiss` — Whether to show the dismiss button (default: true).
-->
<script lang="ts">
  import { formatAccountName } from '$lib/render/accountName';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
  import { createQuery, skipToken } from '@tanstack/svelte-query';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { MessageLink } from '$lib/messageLinks';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import {
    fetchMessagePreview,
    messagePreviewQueryKey,
    withRefreshedPreviewUrls,
    type MessagePreview,
    type MessagePreviewAttachment
  } from '$lib/query/messagePreview';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { createAttachmentAPI } from '$lib/api-client/attachments';
  import {
    assetUrlNeedsRefresh,
    earliestAssetUrlRefreshAt,
    refreshAttachmentUrlsForAssets,
    withAssetUrlRetryParam
  } from '$lib/attachments/attachmentUrls';
  import { useExpiringAssetUrlRefresh } from '$lib/attachments/useExpiringAssetUrlRefresh.svelte';
  import { ScrollFader } from '$lib/ui';
  import MessageContent from './MessageContent.svelte';
  import UserAvatar from './UserAvatar.svelte';
  import DeletedUserLabel from './DeletedUserLabel.svelte';

  let {
    link,
    onDismiss,
    showDismiss = true
  }: {
    link: MessageLink;
    onDismiss?: () => void;
    showDismiss?: boolean;
  } = $props();

  const thumbnailRetrySalts = new SvelteMap<string, number>();
  let refreshPromise: Promise<void> | null = null;
  const failedThumbnailRefreshes = new SvelteSet<string>();
  const brokenThumbnailIds = new SvelteSet<string>();
  const PREVIEW_THUMBNAIL_REFRESH = {
    width: 120,
    height: 120,
    fit: ImageFitMode.COVER
  };

  // A registered server always has a store and a connection. Sign-out and
  // account changes replace both, and the new connection has a new query scope.
  const store = $derived(link.serverId ? serverRegistry.tryGetStore(link.serverId) : undefined);
  const connection = $derived(
    store && link.serverId ? serverConnectionManager.getClient(link.serverId) : undefined
  );
  const queryKey = $derived(
    connection && link.serverId
      ? messagePreviewQueryKey(link.serverId, connection, link.roomId, link.messageId)
      : ['message-preview', 'unavailable']
  );

  const previewQuery = createQuery(
    () => {
      const { serverId, roomId, messageId } = link;
      const currentStore = store;
      const currentConnection = connection;
      return {
        queryKey,
        queryFn:
          serverId && currentStore && currentConnection
            ? ({ signal }: { signal: AbortSignal }) =>
                fetchMessagePreview(serverId, currentConnection, {
                  roomId,
                  messageId,
                  minimumCursor: currentStore.minimumReadCursor,
                  signal
                })
            : skipToken,
        // The card owns the preview: do not keep it after the card unmounts.
        gcTime: 0
      };
    },
    () => queryClient
  );

  // Thumbnail URLs refreshed for the loaded preview. A reload of the preview
  // brings its own URLs and replaces these.
  let refreshed = $state.raw<{ source: MessagePreview; preview: MessagePreview } | null>(null);

  // Show nothing while loading and when the message can't be loaded (not
  // found, no permission, unknown server).
  const preview = $derived.by((): MessagePreview | null => {
    const loaded = previewQuery.data ?? null;
    return refreshed && refreshed.source === loaded ? refreshed.preview : loaded;
  });

  const spaceName = $derived(
    link.serverId ? (serverRegistry.getServer(link.serverId)?.name ?? null) : null
  );
  const roomName = $derived(
    store?.navigation.rooms.find((room) => room.id === link.roomId)?.name ?? null
  );

  function previewThumbnailUrl(attachment: MessagePreviewAttachment): string | null {
    if (brokenThumbnailIds.has(attachment.id)) return null;
    const thumbnailAssetUrl = attachment.contentType.startsWith('video/')
      ? (attachment.videoThumbnailAssetUrl ?? attachment.thumbnailAssetUrl)
      : attachment.thumbnailAssetUrl;
    if (!thumbnailAssetUrl) return null;
    const salt = thumbnailRetrySalts.get(attachment.id);
    return salt ? withAssetUrlRetryParam(thumbnailAssetUrl.url, salt) : thumbnailAssetUrl.url;
  }

  const displayName = $derived(
    preview?.actor
      ? getLiveDisplayName(preview.actor.id, preview.actor.displayName || preview.actor.login)
      : null
  );

  const bodyMarkdown = $derived(preview?.body ?? '');
  const hasBody = $derived(bodyMarkdown.trim().length > 0);

  function attachmentLabel(contentType: string): string {
    if (contentType.startsWith('image/')) return m('message_preview.attachment_image');
    if (contentType.startsWith('video/')) return m('message_preview.attachment_video');
    if (contentType.startsWith('audio/')) return m('message_preview.attachment_audio');
    return m('message_preview.attachment_file');
  }

  const nextThumbnailRefreshAt = $derived.by(() =>
    earliestAssetUrlRefreshAt(
      preview?.attachments.flatMap((a) => [a.thumbnailAssetUrl, a.videoThumbnailAssetUrl]) ?? []
    )
  );

  function hasStaleThumbnailUrl() {
    return (
      preview?.attachments.some(
        (attachment) =>
          assetUrlNeedsRefresh(attachment.thumbnailAssetUrl) ||
          assetUrlNeedsRefresh(attachment.videoThumbnailAssetUrl)
      ) ?? false
    );
  }

  async function refreshPreviewAttachmentUrls(): Promise<void> {
    const source = previewQuery.data;
    if (!preview || !source || refreshPromise) return refreshPromise ?? undefined;
    if (!connection || !link.serverId) return undefined;

    const current = preview;
    const serverId = link.serverId;
    refreshPromise = refreshAttachmentUrlsForAssets(
      connection.getAPI(createAttachmentAPI),
      link.roomId,
      current.attachments.map((attachment) => attachment.id),
      PREVIEW_THUMBNAIL_REFRESH
    )
      .then((freshUrls) => {
        // Ignore URLs for a preview that was reloaded or replaced meanwhile.
        if (freshUrls.size === 0 || previewQuery.data !== source) return;
        refreshed = { source, preview: withRefreshedPreviewUrls(serverId, current, freshUrls) };
      })
      .catch(() => {
        // Fail silently — the preview can still render text and file labels.
      })
      .finally(() => {
        refreshPromise = null;
      });

    return refreshPromise;
  }

  function refreshAfterThumbnailError(attachment: MessagePreviewAttachment) {
    if (failedThumbnailRefreshes.has(attachment.id)) {
      brokenThumbnailIds.add(attachment.id);
      return;
    }
    failedThumbnailRefreshes.add(attachment.id);
    refreshPreviewAttachmentUrls().then(() => {
      thumbnailRetrySalts.set(attachment.id, Date.now());
    });
  }

  function openPreview(event: MouseEvent) {
    if (!preview) return;
    if (event.defaultPrevented) return;

    const target = event.target as HTMLElement;
    if (target.closest('a, button')) return;

    navigateToPreview();
  }

  function handlePreviewKeydown(event: KeyboardEvent) {
    if (!preview) return;
    if (event.target !== event.currentTarget) return;
    if (event.key !== 'Enter' && event.key !== ' ') return;

    event.preventDefault();
    navigateToPreview();
  }

  function navigateToPreview() {
    if (!preview || !link.serverId) return;
    const serverId = serverIdToSegment(link.serverId);
    if (link.threadRootEventId) {
      goto(
        resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
          serverId,
          roomId: link.roomId,
          threadId: link.threadRootEventId,
          messageId: link.messageId
        })
      );
      return;
    }

    goto(
      resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        serverId,
        roomId: link.roomId,
        messageId: link.messageId
      })
    );
  }

  useExpiringAssetUrlRefresh({
    getRefreshAt: () => nextThumbnailRefreshAt,
    hasStaleUrl: hasStaleThumbnailUrl,
    refresh: refreshPreviewAttachmentUrls,
    errorMessage: 'Failed to refresh message preview attachment URLs'
  });
</script>

{#if preview}
  <div
    role="link"
    tabindex="0"
    aria-label={`Open linked message${displayName ? ` from ${formatAccountName(displayName, preview.actor)}` : ''}`}
    data-testid="message-preview-card"
    class="group/preview relative embed-frame flex w-full max-w-[min(42rem,100%)] cursor-pointer flex-col"
    onclick={openPreview}
    onkeydown={handlePreviewKeydown}
  >
    <div class="flex min-w-0 flex-col">
      <div
        class="flex min-w-0 items-start gap-2 border-b border-border/70 bg-surface-emphasized/60 px-3 py-2"
      >
        <div class="mt-1 h-8 w-1 shrink-0 rounded-full bg-action/70"></div>
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          {#if spaceName || roomName}
            <span class="truncate text-xs tracking-wide text-muted">
              {#if spaceName}<bdi>{spaceName}</bdi>{/if}
              {#if spaceName && roomName}&nbsp;·&nbsp;{/if}
              {#if roomName}<bdi>#{roomName}</bdi>{/if}
            </span>
          {/if}
          <div class="flex min-w-0 items-center gap-2">
            {#if preview.actor && !preview.actor.deleted}
              <UserAvatar user={preview.actor} size="xs" />
              <AccountName
                name={displayName ?? ''}
                identity={preview.actor}
                class="text-sm font-medium"
              />
            {:else}
              <span class="truncate text-sm font-medium text-muted"><DeletedUserLabel /></span>
            {/if}
          </div>
        </div>
      </div>
      {#if hasBody}
        <ScrollFader
          top
          bottom
          fill={false}
          fadeHeight="h-5"
          fadeColorClass="from-surface via-surface/80"
          scrollClass="max-h-52 overscroll-contain"
        >
          <div class="px-3 py-2.5 text-sm leading-relaxed pointer-fine:select-text">
            <MessageContent body={bodyMarkdown} viewerLogin={store?.currentUser.user?.login} />
          </div>
        </ScrollFader>
      {/if}
      {#if preview.attachments.length > 0}
        <div
          class={[
            'flex items-center gap-2 border-t border-border/70 px-3 py-2',
            hasBody ? 'bg-surface/60' : ''
          ]}
        >
          {#each preview.attachments.slice(0, 4) as attachment (attachment.id)}
            {@const thumbnailUrl = previewThumbnailUrl(attachment)}
            {#if thumbnailUrl}
              <div
                class="relative h-12 w-12 shrink-0 overflow-hidden rounded-sm border border-border"
              >
                <img
                  src={thumbnailUrl}
                  alt={attachment.description || attachment.filename}
                  class="h-full w-full object-cover"
                  onerror={() => refreshAfterThumbnailError(attachment)}
                />
                {#if attachment.contentType.startsWith('video/')}
                  <span
                    class="absolute inset-0 flex items-center justify-center bg-black/15 text-white"
                    aria-hidden="true"
                  >
                    <span
                      class="iconify icon-[uil--play] flex h-6 w-6 items-center justify-center rounded-full bg-black/55 text-sm shadow-sm"
                    ></span>
                  </span>
                {/if}
              </div>
            {:else}
              <div
                class="flex h-12 w-12 items-center justify-center rounded-sm border border-border bg-surface-emphasized text-xs text-muted"
              >
                {#if attachment.contentType.startsWith('video/')}
                  <span
                    class="iconify icon-[uil--play] flex h-6 w-6 items-center justify-center rounded-full bg-black/45 text-sm text-white shadow-sm"
                    aria-hidden="true"
                  ></span>
                {:else}
                  {attachmentLabel(attachment.contentType)}
                {/if}
              </div>
            {/if}
          {/each}
          {#if preview.attachments.length > 4}
            <span class="text-xs text-muted">+{preview.attachments.length - 4}</span>
          {/if}
          {#if !hasBody}
            <span class="text-xs text-muted">
              {preview.attachments.length === 1
                ? attachmentLabel(preview.attachments[0].contentType)
                : m('message_preview.attachments_count', {
                    count: preview.attachments.length
                  })}
            </span>
          {/if}
        </div>
      {/if}
    </div>
    {#if showDismiss && onDismiss}
      <button
        type="button"
        onclick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onDismiss?.();
        }}
        class="embed-control-button"
        aria-label={m('preview.dismiss')}
      >
        <span class="iconify icon-[uil--times] text-sm"></span>
      </button>
    {/if}
  </div>
{/if}
