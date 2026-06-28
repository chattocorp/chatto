<!--
@component

Displays a preview card for a Chatto message link (e.g. pasted in the composer
or embedded in a posted message). The message is fetched through the appropriate
instance's Connect timeline API; if it can't be loaded (not found, no permission,
unknown instance) the component renders nothing.

**Props:**
- `link` — Parsed MessageLink from `$lib/messageLinks`.
- `onDismiss` — Callback when user dismisses the preview (composer mode).
- `showDismiss` — Whether to show the dismiss button (default: true).
-->
<script lang="ts">
  import { goto } from '$app/navigation';
  import type { MessageLink } from '$lib/messageLinks';
  import {
    FitMode,
    MessageAttachmentViewDocument,
    type MessageAttachmentView,
    type UserAvatarUserView
  } from '$lib/render/types';
  import { useRenderData } from '$lib/render/data';
  import { resolve } from '$app/paths';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { serverIdToSegment } from '$lib/navigation';
  import * as m from '$lib/i18n/messages';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { createRoomTimelineAPI } from '$lib/api/roomTimeline';
  import { createAttachmentAPI } from '$lib/api/attachments';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import { unmask } from '$lib/state/room/messages/helpers';
  import {
    assetUrlNeedsRefresh,
    earliestAssetUrlRefreshAt,
    refreshAttachmentUrlsForMessage,
    withAssetUrlRetryParam,
    type ExpiringAssetUrl
  } from '$lib/attachments/attachmentUrls';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import MessageContent from './MessageContent.svelte';
  import UserAvatar, { UserAvatarViewData } from './UserAvatar.svelte';

  let {
    link,
    onDismiss,
    showDismiss = true
  }: {
    link: MessageLink;
    onDismiss?: () => void;
    showDismiss?: boolean;
  } = $props();

  interface Attachment {
    id: string;
    filename: string;
    contentType: string;
    thumbnailAssetUrl: ExpiringAssetUrl | null;
    videoThumbnailAssetUrl: ExpiringAssetUrl | null;
    thumbnailUrl: string | null;
  }

  let preview = $state<{
    serverId: string;
    roomId: string;
    eventId: string;
    body: string | null;
    attachments: Attachment[];
    actor: UserAvatarUserView | null;
    spaceName: string | null;
    roomName: string | null;
  } | null>(null);
  const thumbnailRetrySalts = new SvelteMap<string, number>();
  let refreshPromise: Promise<void> | null = null;
  const failedThumbnailRefreshes = new SvelteSet<string>();
  const brokenThumbnailIds = new SvelteSet<string>();
  const PREVIEW_THUMBNAIL_REFRESH = {
    width: 120,
    height: 120,
    fit: FitMode.Cover
  };

  function connectBaseUrl(serverUrl: string): string {
    return new URL('/api/connect', serverUrl).toString();
  }

  function roomName(serverId: string, roomId: string): string | null {
    return (
      serverRegistry.tryGetStore(serverId)?.rooms.rooms.find((room) => room.id === roomId)?.name ??
      null
    );
  }

  function normalizePreviewAssetUrl(
    serverId: string,
    value: ExpiringAssetUrl | null | undefined
  ): ExpiringAssetUrl | null {
    if (!value) return null;
    return {
      ...value,
      url: assetUrlForServer(serverId, value.url) ?? value.url
    };
  }

  function previewThumbnailUrl(attachment: Attachment): string | null {
    if (brokenThumbnailIds.has(attachment.id)) return null;
    const thumbnailAssetUrl = attachment.contentType.startsWith('video/')
      ? (attachment.videoThumbnailAssetUrl ?? attachment.thumbnailAssetUrl)
      : attachment.thumbnailAssetUrl;
    if (!thumbnailAssetUrl) return null;
    const salt = thumbnailRetrySalts.get(attachment.id);
    return salt ? withAssetUrlRetryParam(thumbnailAssetUrl.url, salt) : thumbnailAssetUrl.url;
  }

  $effect(() => {
    const { serverId, roomId, messageId } = link;

    preview = null;
    if (!serverId) return;

    let cancelled = false;

    (async () => {
      try {
        const server = serverRegistry.getServer(serverId);
        if (!server) return;
        const page = await createRoomTimelineAPI({
          serverId,
          baseUrl: connectBaseUrl(server.url),
          bearerToken: server.token
        }).getRoomEventsAround({
          roomId,
          eventId: messageId,
          limit: 1
        });

        if (cancelled) return;

        const ev = unmask(page.events).find((item) => item.id === messageId);
        const inner = ev?.event;
        if (!ev || !isMessagePostedEvent(inner)) {
          return;
        }

        const attachments = inner.attachments.map((attachment) =>
          useRenderData(MessageAttachmentViewDocument, attachment)
        );

        // Need at least a body or attachments for a meaningful preview
        if (!inner.body && attachments.length === 0) {
          return;
        }

        preview = {
          serverId,
          roomId,
          eventId: messageId,
          body: inner.body ?? null,
          attachments: attachments.map((a: MessageAttachmentView) => {
            const thumbnailAssetUrl = normalizePreviewAssetUrl(serverId, a.thumbnailAssetUrl);
            const videoThumbnailAssetUrl = normalizePreviewAssetUrl(
              serverId,
              a.videoProcessing?.thumbnailAssetUrl
            );
            const displayThumbnailAssetUrl = a.contentType.startsWith('video/')
              ? (videoThumbnailAssetUrl ?? thumbnailAssetUrl)
              : thumbnailAssetUrl;
            return {
              id: a.id,
              filename: a.filename,
              contentType: a.contentType,
              thumbnailAssetUrl,
              videoThumbnailAssetUrl,
              thumbnailUrl: displayThumbnailAssetUrl?.url ?? null
            };
          }),
          actor: ev.actor ? useRenderData(UserAvatarViewData, ev.actor) : null,
          spaceName: server.name ?? null,
          roomName: roomName(serverId, roomId)
        };
      } catch {
        // Fail silently — no preview shown.
      }
    })();

    return () => {
      cancelled = true;
    };
  });

  const displayName = $derived(
    preview?.actor
      ? getLiveDisplayName(preview.actor.id, preview.actor.displayName || preview.actor.login)
      : null
  );

  const bodyMarkdown = $derived(preview?.body ?? '');
  const hasBody = $derived(bodyMarkdown.trim().length > 0);

  function attachmentLabel(contentType: string): string {
    if (contentType.startsWith('image/')) return 'Image';
    if (contentType.startsWith('video/')) return 'Video';
    if (contentType.startsWith('audio/')) return 'Audio';
    return 'File';
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
    if (!preview || refreshPromise) return refreshPromise ?? undefined;

    const current = preview;
    const server = serverRegistry.getServer(current.serverId);
    if (!server) return undefined;
    refreshPromise = refreshAttachmentUrlsForMessage(
      createAttachmentAPI({
        serverId: current.serverId,
        baseUrl: connectBaseUrl(server.url),
        bearerToken: server.token
      }),
      current.roomId,
      current.eventId,
      PREVIEW_THUMBNAIL_REFRESH
    )
      .then((freshUrls) => {
        if (freshUrls.size === 0) return;
        if (
          !preview ||
          preview.serverId !== current.serverId ||
          preview.roomId !== current.roomId ||
          preview.eventId !== current.eventId
        ) {
          return;
        }

        preview = {
          ...preview,
          attachments: preview.attachments.map((attachment) => {
            const freshThumbnail = normalizePreviewAssetUrl(
              current.serverId,
              freshUrls.get(attachment.id)?.thumbnailAssetUrl
            );
            const freshVideoThumbnail = normalizePreviewAssetUrl(
              current.serverId,
              freshUrls.get(attachment.id)?.videoThumbnailAssetUrl
            );
            if (!freshThumbnail && !freshVideoThumbnail) return attachment;

            const thumbnailAssetUrl = freshThumbnail ?? attachment.thumbnailAssetUrl;
            const videoThumbnailAssetUrl = freshVideoThumbnail ?? attachment.videoThumbnailAssetUrl;
            const displayThumbnailAssetUrl = attachment.contentType.startsWith('video/')
              ? (videoThumbnailAssetUrl ?? thumbnailAssetUrl)
              : thumbnailAssetUrl;

            return {
              ...attachment,
              thumbnailAssetUrl,
              videoThumbnailAssetUrl,
              thumbnailUrl: displayThumbnailAssetUrl?.url ?? null
            };
          })
        };
      })
      .catch(() => {
        // Fail silently — the preview can still render text and file labels.
      })
      .finally(() => {
        refreshPromise = null;
      });

    return refreshPromise;
  }

  function refreshAfterThumbnailError(attachment: Attachment) {
    if (failedThumbnailRefreshes.has(attachment.id)) {
      brokenThumbnailIds.add(attachment.id);
      return;
    }
    failedThumbnailRefreshes.add(attachment.id);
    refreshPreviewAttachmentUrls().then(() => {
      thumbnailRetrySalts.set(attachment.id, Date.now());
    });
  }

  function refreshStalePreviewUrls() {
    if (hasStaleThumbnailUrl()) {
      refreshPreviewAttachmentUrls();
    }
  }

  function handleVisibilityChange() {
    if (document.visibilityState === 'visible') {
      refreshStalePreviewUrls();
    }
  }

  function openPreview(event: MouseEvent) {
    if (!preview) return;
    if (event.defaultPrevented) return;

    const target = event.target as HTMLElement;
    if (target.closest('a, button')) return;

    goto(
      resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        serverId: serverIdToSegment(preview.serverId),
        roomId: preview.roomId,
        messageId: preview.eventId
      })
    );
  }

  function handlePreviewKeydown(event: KeyboardEvent) {
    if (!preview) return;
    if (event.target !== event.currentTarget) return;
    if (event.key !== 'Enter' && event.key !== ' ') return;

    event.preventDefault();
    goto(
      resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        serverId: serverIdToSegment(preview.serverId),
        roomId: preview.roomId,
        messageId: preview.eventId
      })
    );
  }

  $effect(() => {
    if (nextThumbnailRefreshAt === null) return;

    const timeout = window.setTimeout(
      () => {
        refreshPreviewAttachmentUrls();
      },
      Math.max(0, nextThumbnailRefreshAt - Date.now())
    );

    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    refreshStalePreviewUrls();
  });

  $effect(() => {
    window.addEventListener('focus', refreshStalePreviewUrls);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      window.removeEventListener('focus', refreshStalePreviewUrls);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  });
</script>

{#if preview}
  <div
    role="link"
    tabindex="0"
    aria-label={`Open linked message${displayName ? ` from ${displayName}` : ''}`}
    data-testid="message-preview-card"
    class="group/preview relative embed-frame flex w-full max-w-[min(42rem,100%)] cursor-pointer flex-col"
    onclick={openPreview}
    onkeydown={handlePreviewKeydown}
  >
    <div class="flex min-w-0 flex-col">
      <div
        class="flex min-w-0 items-start gap-2 border-b border-border/70 bg-surface-200/60 px-3 py-2"
      >
        <div class="mt-1 h-8 w-1 shrink-0 rounded-full bg-accent/70"></div>
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          {#if preview.spaceName || preview.roomName}
            <span class="truncate text-xs tracking-wide text-muted">
              {#if preview.spaceName}{preview.spaceName}{/if}
              {#if preview.spaceName && preview.roomName}&nbsp;·&nbsp;{/if}
              {#if preview.roomName}#{preview.roomName}{/if}
            </span>
          {/if}
          <div class="flex min-w-0 items-center gap-2">
            {#if preview.actor}
              <UserAvatar user={preview.actor} size="xs" />
              <span class="truncate text-sm font-medium">{displayName}</span>
            {:else}
              <span class="truncate text-sm font-medium text-muted">
                {m['message_preview.deleted_user']()}
              </span>
            {/if}
          </div>
        </div>
      </div>
      {#if hasBody}
        <div class="relative">
          <div
            class="pointer-events-none absolute inset-x-0 top-0 z-10 h-5 bg-gradient-to-b from-surface-100 via-surface-100/80 to-transparent"
            aria-hidden="true"
          ></div>
          <div
            class="pointer-events-none absolute inset-x-0 bottom-0 z-10 h-5 bg-gradient-to-t from-surface-100 via-surface-100/80 to-transparent"
            aria-hidden="true"
          ></div>
          <div
            class="max-h-52 overflow-y-auto overscroll-contain px-3 py-2.5 text-sm leading-relaxed pointer-fine:select-text"
          >
            <MessageContent body={bodyMarkdown} />
          </div>
        </div>
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
                  alt={attachment.filename}
                  class="h-full w-full object-cover"
                  onerror={() => refreshAfterThumbnailError(attachment)}
                />
                {#if attachment.contentType.startsWith('video/')}
                  <span
                    class="absolute inset-0 flex items-center justify-center bg-black/15 text-white"
                    aria-hidden="true"
                  >
                    <span
                      class="iconify flex h-6 w-6 items-center justify-center rounded-full bg-black/55 text-sm shadow-sm uil--play"
                    ></span>
                  </span>
                {/if}
              </div>
            {:else}
              <div
                class="flex h-12 w-12 items-center justify-center rounded-sm border border-border bg-surface-200 text-xs text-muted"
              >
                {#if attachment.contentType.startsWith('video/')}
                  <span
                    class="iconify flex h-6 w-6 items-center justify-center rounded-full bg-black/45 text-sm text-white shadow-sm uil--play"
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
                : m['message_preview.attachments_count']({
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
        class="embed-control-button md:group-hover/preview:opacity-100"
        aria-label={m['preview.dismiss']()}
      >
        <span class="iconify text-sm uil--times"></span>
      </button>
    {/if}
  </div>
{/if}
