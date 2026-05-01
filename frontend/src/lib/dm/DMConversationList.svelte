<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { untrack } from 'svelte';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import InstancePill from '$lib/components/InstancePill.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import {
    getDMConversationsStore,
    type DMConversation
  } from '$lib/state/dm/conversations.svelte';

  let {
    activeConversationId
  }: {
    activeConversationId?: string;
  } = $props();

  const store = getDMConversationsStore();

  // Clear the unread flag and dismiss DM notifications when the user enters a
  // conversation. The find + writes are wrapped in `untrack` so this effect
  // depends only on `activeConversationId` — without that, the `markRead`
  // write feeds back into the `find` read and trips effect_update_depth_exceeded.
  // Late-loaded conversations are handled in the store's loadInstance, which
  // pre-clears `hasUnread` for the active conv; Room.svelte:dismissDMNotifications
  // handles the dismissal path when the conversation page actually mounts.
  $effect(() => {
    if (!activeConversationId) return;
    untrack(() => {
      const conv = store.conversations.find((c) => c.id === activeConversationId);
      if (!conv) return;
      store.markRead(conv.instanceId, conv.id);
      const stores = instanceRegistry.tryGetStore(conv.instanceId);
      void stores?.notifications.dismissDMNotifications(conv.id);
    });
  });

  function getConversationDisplayName(conv: DMConversation): string {
    if (conv.isSelfConversation) {
      const self = conv.participants.find((p) => p.id === conv.currentUserId);
      if (self) return getLiveDisplayName(self.id, self.displayName || self.login);
      return 'You';
    }
    const others = conv.participants.filter((p) => p.id !== conv.currentUserId);
    return others.map((p) => getLiveDisplayName(p.id, p.displayName || p.login)).join(', ');
  }

  // Whether a given conversation has a pending DM notification on its instance.
  function convHasNotification(conv: DMConversation): boolean {
    return (
      instanceRegistry.tryGetStore(conv.instanceId)?.notifications.hasDMRoomNotification(conv.id) ??
      false
    );
  }

  async function handleDMNotificationClick(event: MouseEvent, conv: DMConversation) {
    event.preventDefault();
    event.stopPropagation();

    const stores = instanceRegistry.tryGetStore(conv.instanceId);
    if (!stores) return;
    const notification = stores.notifications.getDMRoomNotification(conv.id);
    if (!notification) return;

    void stores.notifications.dismiss(notification.id);

    const path = stores.notifications.getCleanPath(conv.instanceId, notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }

  // Whether we're connected to multiple instances (controls instance label display)
  let multiInstance = $derived(instanceRegistry.instances.length > 1);
</script>

<nav class="sidebar-nav w-80 overflow-y-auto p-2">
  {#if store.isLoading && store.conversations.length === 0}
    <div class="flex items-center justify-center p-4">
      <span class="iconify animate-spin text-xl text-text/50 uil--spinner-alt"></span>
    </div>
  {:else if store.conversations.length === 0}
    <p class="p-4 text-center text-sm text-text/50">No conversations yet</p>
  {:else}
    {#each store.conversations as conv (`${conv.instanceId}:${conv.id}`)}
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

        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <span class="truncate">{getConversationDisplayName(conv)}</span>
          {#if multiInstance}
            <InstancePill instanceId={conv.instanceId} />
          {/if}
        </div>

        {#if convHasNotification(conv)}
          <button
            type="button"
            onclick={(e) => handleDMNotificationClick(e, conv)}
            class="-mr-2 flex h-6 w-6 cursor-pointer items-center justify-center notification-dot"
            aria-label="Go to notification"
          >
            <UnreadDot />
          </button>
          <span class="sr-only">new direct message</span>
        {:else if conv.hasUnread}
          <UnreadDot color="primary" testid="dm-unread-dot" />
          <span class="sr-only">unread messages</span>
        {/if}
      </a>
    {/each}
  {/if}
</nav>
