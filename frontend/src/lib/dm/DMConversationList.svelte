<script lang="ts">
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { instanceEventBusManager } from '$lib/state/instance/eventBus.svelte';
  import { graphql, useFragment } from '$lib/gql';
  import {
    RoomEventViewFragmentDoc,
    SpaceEventBusSubscriptionDocument,
    UserAvatarUserFragmentDoc,
    type UserAvatarUserFragment
  } from '$lib/gql/graphql';
  import { DM_SPACE_ID } from '$lib/constants';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import type { EventHandler } from '$lib/instanceEventBus.svelte';

  let {
    activeConversationId
  }: {
    activeConversationId?: string;
  } = $props();

  type DMConversation = {
    id: string;
    instanceId: string;
    instanceLabel: string;
    hasUnread: boolean;
    participants: UserAvatarUserFragment[];
    currentUserId: string | undefined;
    isSelfConversation: boolean;
  };

  let conversations = $state<DMConversation[]>([]);
  let loadingCount = $state(0);
  let loading = $derived(loadingCount > 0);

  const DMConversationsQuery = graphql(`
    query GetDmConversationsForList {
      me {
        id
      }
      space(id: "DM") {
        rooms {
          id
          hasUnread
          members {
            ...UserAvatarUser
          }
        }
      }
    }
  `);

  function getInstanceHostname(instance: { url: string }): string {
    try {
      return new URL(instance.url).hostname;
    } catch {
      return instance.url;
    }
  }

  /** Load DM conversations from a single instance and merge into the list. */
  async function loadInstanceConversations(instanceId: string): Promise<void> {
    const instance = instanceRegistry.getInstance(instanceId);
    if (!instance) return;

    const client = graphqlClientManager.getClient(instanceId);
    const result = await client.client.query(DMConversationsQuery, {}).toPromise();
    if (!result.data?.space) return;

    const me = result.data.me;
    const meId = me?.id;
    const label = instance.name?.trim() || getInstanceHostname(instance);
    const rooms = result.data.space.rooms ?? [];

    const newConversations: DMConversation[] = rooms.map((room) => {
      const participants = room.members.map((m) =>
        useFragment(UserAvatarUserFragmentDoc, m)
      );
      const others = participants.filter((p) => p.id !== meId);
      return {
        id: room.id,
        instanceId,
        instanceLabel: label,
        hasUnread: room.hasUnread,
        participants,
        currentUserId: meId,
        isSelfConversation: others.length === 0
      };
    });

    // Replace this instance's conversations in the merged list
    conversations = [
      ...conversations.filter((c) => c.instanceId !== instanceId),
      ...newConversations
    ];
  }

  /** Load conversations from all connected instances in parallel. */
  async function loadAllConversations() {
    const instances = instanceRegistry.instances;
    loadingCount = instances.length;

    await Promise.allSettled(
      instances.map(async (instance) => {
        try {
          await loadInstanceConversations(instance.id);
        } finally {
          loadingCount--;
        }
      })
    );
  }

  function getConversationDisplayName(conv: DMConversation): string {
    if (conv.isSelfConversation) {
      const self = conv.participants.find((p) => p.id === conv.currentUserId);
      if (self) {
        return getLiveDisplayName(self.id, self.displayName || self.login);
      }
      return 'You';
    }
    const others = conv.participants.filter((p) => p.id !== conv.currentUserId);
    return others.map((p) => getLiveDisplayName(p.id, p.displayName || p.login)).join(', ');
  }

  // Track rooms with in-flight refetches to prevent duplicate requests
  let pendingRefetch = new SvelteSet<string>();

  function bumpConversationToTop(instanceId: string, roomId: string, markUnread: boolean) {
    const index = conversations.findIndex((c) => c.id === roomId && c.instanceId === instanceId);
    if (index === -1) {
      const key = `${instanceId}:${roomId}`;
      if (!pendingRefetch.has(key)) {
        pendingRefetch.add(key);
        loadInstanceConversations(instanceId).then(() => {
          pendingRefetch.delete(key);
          bumpConversationToTop(instanceId, roomId, markUnread);
        });
      }
      return;
    }

    const conv = conversations[index];
    if (markUnread) {
      conv.hasUnread = true;
    }

    if (index > 0) {
      conversations = [conv, ...conversations.slice(0, index), ...conversations.slice(index + 1)];
    }
  }

  // Load conversations from all instances on mount
  $effect(() => {
    loadAllConversations();
  });

  // Clear unread status when entering a conversation
  $effect(() => {
    if (activeConversationId) {
      const conv = conversations.find((c) => c.id === activeConversationId);
      if (conv) conv.hasUnread = false;
    }
  });

  // Subscribe to DM notifications from ALL instance event buses.
  // This replaces the old useSpaceEvent + useNewDM pattern which required
  // being inside a specific instance's context.
  $effect(() => {
    const cleanups: (() => void)[] = [];

    for (const instance of instanceRegistry.instances) {
      const bus = instanceEventBusManager.getBus(instance.id);
      if (!bus) continue;

      const handler: EventHandler = (event) => {
        if (event.event?.__typename === 'NewDirectMessageNotificationEvent') {
          bumpConversationToTop(
            instance.id,
            event.event.roomId,
            event.event.roomId !== activeConversationId
          );
        }
      };

      bus.handlers.add(handler);
      cleanups.push(() => bus.handlers.delete(handler));
    }

    return () => cleanups.forEach((c) => c());
  });

  // Subscribe to DM space events (MessagePostedEvent) per instance.
  // This catches ALL messages (including your own) so the sidebar updates
  // immediately when you send a message — not just when you receive one.
  $effect(() => {
    const subs: (() => void)[] = [];

    for (const instance of instanceRegistry.instances) {
      const client = graphqlClientManager.getClient(instance.id);
      const sub = client.client
        .subscription(SpaceEventBusSubscriptionDocument, { spaceId: DM_SPACE_ID })
        .subscribe((result) => {
          if (!result.data) return;
          const event = useFragment(RoomEventViewFragmentDoc, result.data.mySpaceEvents);
          if (event?.event?.__typename === 'MessagePostedEvent') {
            bumpConversationToTop(
              instance.id,
              event.event.roomId,
              event.event.roomId !== activeConversationId
            );
          }
        });
      subs.push(() => sub.unsubscribe());
    }

    return () => subs.forEach((s) => s());
  });

  // Whether we're connected to multiple instances (controls instance label display)
  let multiInstance = $derived(instanceRegistry.instances.length > 1);
</script>

<nav class="sidebar-nav w-80 overflow-y-auto p-2">
  {#if loading && conversations.length === 0}
    <div class="flex items-center justify-center p-4">
      <span class="iconify animate-spin text-xl text-text/50 uil--spinner-alt"></span>
    </div>
  {:else if conversations.length === 0}
    <p class="p-4 text-center text-sm text-text/50">No conversations yet</p>
  {:else}
    {#each conversations as conv (`${conv.instanceId}:${conv.id}`)}
      <a
        href={resolve('/chat/dm/[instanceSegment]/[conversationId]', { instanceSegment: instanceIdToSegment(conv.instanceId), conversationId: conv.id })}
        class={[
          'sidebar-item py-3',
          conv.id === activeConversationId ? 'bg-surface-100' : '',
          conv.hasUnread && conv.id !== activeConversationId ? 'font-semibold' : ''
        ]}
      >
        <!-- Avatar -->
        <div class="flex -space-x-2">
          {#if conv.isSelfConversation}
            {#each conv.participants.filter((p) => p.id === conv.currentUserId).slice(0, 1) as participant (participant.id)}
              <UserAvatar user={participant} size="md" />
            {/each}
          {:else}
            {#each conv.participants
              .filter((p) => p.id !== conv.currentUserId)
              .slice(0, 3) as participant (participant.id)}
              <UserAvatar user={participant} size="md" />
            {/each}
          {/if}
        </div>

        <div class="flex min-w-0 flex-1 flex-col">
          <span class="truncate">{getConversationDisplayName(conv)}</span>
          {#if multiInstance}
            <span class="truncate text-xs text-muted" title={conv.instanceLabel}>
              {conv.instanceLabel}
            </span>
          {/if}
        </div>

        <!-- Unread Indicator -->
        {#if conv.hasUnread}
          <UnreadDot />
        {/if}
      </a>
    {/each}
  {/if}
</nav>
