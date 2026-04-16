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

  export const MessagePreviewQuery = graphql(`
    query MessagePreview($spaceId: ID!, $roomId: ID!, $eventId: ID!) {
      roomEventByEventId(spaceId: $spaceId, roomId: $roomId, eventId: $eventId) {
        id
        createdAt
        actor {
          id
          login
          displayName
          avatarUrl(width: 96, height: 96)
        }
        event {
          __typename
          ... on MessagePostedEvent {
            body
            inThread
          }
        }
      }
      space(id: $spaceId) {
        id
        name
      }
      room(spaceId: $spaceId, roomId: $roomId) {
        id
        name
      }
    }
  `);
</script>

<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- href built via buildMessageLinkPath which already calls resolve() */
  import type { MessageLink } from '$lib/messageLinks';
  import { buildMessageLinkPath } from '$lib/messageLinks';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import type { UserAvatarUserFragment } from '$lib/gql/graphql';
  import UserAvatar from './UserAvatar.svelte';

  interface Preview {
    messageId: string;
    path: string;
    body: string;
    createdAt: string;
    actor: {
      id: string;
      login: string;
      displayName: string | null | undefined;
      avatarUrl: string | null | undefined;
    } | null;
    spaceName: string | null;
    roomName: string | null;
  }

  let {
    link,
    onDismiss,
    showDismiss = true
  }: {
    link: MessageLink;
    onDismiss?: () => void;
    showDismiss?: boolean;
  } = $props();

  let preview = $state<Preview | null>(null);

  $effect(() => {
    const instanceId = link.instanceId;
    const spaceId = link.spaceId;
    const roomId = link.roomId;
    const messageId = link.messageId;

    preview = null;

    if (!instanceId) return;

    let cancelled = false;

    (async () => {
      try {
        const client = graphqlClientManager.getClient(instanceId).client;
        const result = await client
          .query(MessagePreviewQuery, { spaceId, roomId, eventId: messageId })
          .toPromise();

        if (cancelled) return;

        const ev = result.data?.roomEventByEventId;
        const inner = ev?.event;
        if (!ev || !inner || inner.__typename !== 'MessagePostedEvent' || !inner.body) {
          return;
        }

        preview = {
          messageId,
          path: buildMessageLinkPath(instanceId, spaceId, roomId, messageId),
          body: inner.body,
          createdAt: ev.createdAt,
          actor: ev.actor
            ? {
                id: ev.actor.id,
                login: ev.actor.login,
                displayName: ev.actor.displayName,
                avatarUrl: ev.actor.avatarUrl
              }
            : null,
          spaceName: result.data?.space?.name ?? null,
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
    preview ? (preview.body.length > 240 ? preview.body.slice(0, 240) + '…' : preview.body) : ''
  );
</script>

{#if preview}
  <a
    href={preview.path}
    data-testid="message-preview-card"
    class="group/preview relative flex max-w-md flex-col embed-frame bg-surface-100 group-hover/msg:bg-surface-200 hover:bg-surface-300"
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
          <UserAvatar
            user={{
              id: preview.actor.id,
              login: preview.actor.login,
              displayName: preview.actor.displayName ?? null,
              avatarUrl: preview.actor.avatarUrl ?? null,
              presenceStatus: 'OFFLINE'
            } as unknown as UserAvatarUserFragment}
            size="xs"
            showPresence={false}
          />
          <span class="shrink-0 text-sm font-medium">{displayName}</span>
        {:else}
          <span class="shrink-0 text-sm font-medium text-muted">Deleted user</span>
        {/if}
      </div>
      <span class="line-clamp-3 text-sm leading-snug whitespace-pre-wrap break-words">
        {bodySnippet}
      </span>
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
