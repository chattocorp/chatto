<!--
@component

Displays a preview card for a Chatto message link (e.g. pasted in the composer
or embedded in a posted message). The message is fetched through the appropriate
instance's GraphQL client; if it can't be loaded (not found, no permission,
unknown instance) the component renders nothing.

**Props:**
- `link` — Parsed MessageLink from `$lib/messageLinks`.
- `onDismiss` — Callback when user dismisses the preview (composer mode).
- `showDismiss` — Whether to show the dismiss button (default: true).
-->
<script lang="ts" module>
  import { graphql } from '$lib/gql';
  import { UserAvatarFragment } from './UserAvatar.svelte';

  export const MessagePreviewQuery = graphql(`
    query MessagePreview($roomId: ID!, $eventId: ID!) {
      roomEventByEventId(roomId: $roomId, eventId: $eventId) {
        id
        createdAt
        actor {
          ...UserAvatarUser
        }
        event {
          __typename
          ... on MessagePostedEvent {
            body
            attachments {
              id
              filename
              contentType
              thumbnailUrl(width: 120, height: 120, fit: COVER)
            }
          }
        }
      }
      server {
        config {
          serverName
        }
      }
      room(roomId: $roomId) {
        id
        name
      }
    }
  `);
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- href built via buildMessageLinkPath which already calls resolve() */
  import type { MessageLink } from '$lib/messageLinks';
  import type { UserAvatarUserFragment } from '$lib/gql/graphql';
  import { useFragment } from '$lib/gql';
  import { buildMessageLinkPath } from '$lib/messageLinks';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

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
    thumbnailUrl: string | null;
  }

  let preview = $state<{
    path: string;
    body: string | null;
    attachments: Attachment[];
    actor: UserAvatarUserFragment | null;
    spaceName: string | null;
    roomName: string | null;
  } | null>(null);

  $effect(() => {
    const { serverId, roomId, messageId } = link;

    preview = null;
    if (!serverId) return;

    // After ADR-027's URL collapse, spaceId is no longer in the link — look
    // up the target instance's primary space ID from the registry instead.
    const spaceId = serverRegistry.tryGetStore(serverId)?.instance.primarySpaceId;
    if (!spaceId) return;

    let cancelled = false;

    (async () => {
      try {
        const client = graphqlClientManager.getClient(serverId).client;
        const result = await client
          .query(MessagePreviewQuery, { roomId, eventId: messageId })
          .toPromise();

        if (cancelled) return;

        const ev = result.data?.roomEventByEventId;
        const inner = ev?.event;
        if (!ev || !inner || inner.__typename !== 'MessagePostedEvent') {
          return;
        }

        // Need at least a body or attachments for a meaningful preview
        if (!inner.body && inner.attachments.length === 0) {
          return;
        }

        preview = {
          path: buildMessageLinkPath(serverId, roomId, messageId),
          body: inner.body ?? null,
          attachments: inner.attachments.map((a) => ({
            id: a.id,
            filename: a.filename,
            contentType: a.contentType,
            thumbnailUrl: a.thumbnailUrl ?? null
          })),
          actor: ev.actor ? useFragment(UserAvatarFragment, ev.actor) : null,
          spaceName: result.data?.server?.config.serverName ?? null,
          roomName: result.data?.room?.name ?? null
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

  const bodySnippet = $derived(
    preview?.body
      ? preview.body.length > 240
        ? preview.body.slice(0, 240) + '…'
        : preview.body
      : ''
  );

  function attachmentLabel(contentType: string): string {
    if (contentType.startsWith('image/')) return 'Image';
    if (contentType.startsWith('video/')) return 'Video';
    if (contentType.startsWith('audio/')) return 'Audio';
    return 'File';
  }
</script>

{#if preview}
  <a
    href={preview.path}
    data-testid="message-preview-card"
    class="group/preview relative flex w-full max-w-md cursor-pointer flex-col embed-frame bg-surface-100 group-hover/msg:bg-surface-200 hover:bg-surface-300"
  >
    <div class="flex min-w-0 flex-col gap-1.5 px-3 py-2.5">
      {#if preview.spaceName || preview.roomName}
        <span class="text-xs tracking-wide text-muted">
          {#if preview.spaceName}{preview.spaceName}{/if}
          {#if preview.spaceName && preview.roomName}&nbsp;·&nbsp;{/if}
          {#if preview.roomName}#{preview.roomName}{/if}
        </span>
      {/if}
      <div class="flex items-center gap-2 min-w-0">
        {#if preview.actor}
          <UserAvatar user={preview.actor} size="xs" showPresence={false} />
          <span class="shrink-0 text-sm font-medium">{displayName}</span>
        {:else}
          <span class="shrink-0 text-sm font-medium text-muted">Deleted user</span>
        {/if}
      </div>
      {#if bodySnippet}
        <span class="line-clamp-3 text-sm leading-snug whitespace-pre-wrap break-words">
          {bodySnippet}
        </span>
      {/if}
      {#if preview.attachments.length > 0}
        <div class="flex items-center gap-2">
          {#each preview.attachments.slice(0, 3) as attachment (attachment.id)}
            {#if attachment.thumbnailUrl}
              <img
                src={attachment.thumbnailUrl}
                alt={attachment.filename}
                class="h-10 w-10 rounded object-cover"
              />
            {:else}
              <div class="flex h-10 w-10 items-center justify-center rounded bg-surface-200 text-xs text-muted">
                {attachmentLabel(attachment.contentType)}
              </div>
            {/if}
          {/each}
          {#if preview.attachments.length > 3}
            <span class="text-xs text-muted">+{preview.attachments.length - 3}</span>
          {/if}
          {#if !bodySnippet}
            <span class="text-xs text-muted">
              {preview.attachments.length === 1
                ? attachmentLabel(preview.attachments[0].contentType)
                : `${preview.attachments.length} attachments`}
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
        class="absolute top-1 right-1 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full bg-black/60 text-white shadow-md ring-1 ring-white/30 transition-opacity hover:bg-black/80 md:opacity-0 md:group-hover/preview:opacity-100"
        aria-label="Dismiss preview"
      >
        <span class="iconify text-sm uil--times"></span>
      </button>
    {/if}
  </a>
{/if}
