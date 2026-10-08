<!--
@component

The **Home** page: what needs the viewer on every signed-in server. It shows
calls in progress, direct messages, and the Notification Feed.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/client';
  import { serverUi } from '$lib/state/server/serverUi';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import NotificationFeed from '$lib/components/notifications/NotificationFeed.svelte';
  import HomeDirectMessages from '$lib/components/home/HomeDirectMessages.svelte';
  import HomeLiveCalls from '$lib/components/home/HomeLiveCalls.svelte';
  import {
    homeDirectMessages,
    homeDirectMessagesLoading,
    homeLiveCalls,
    type HomeServerSource
  } from '$lib/home/homeOverview';

  const sources = $derived(
    serverRegistry.servers.flatMap((server): HomeServerSource[] => {
      const stores = serverRegistry.tryGetStore(server.id);
      if (!stores?.isAuthenticated) return [];
      const ui = serverUi(stores);
      return [
        {
          serverId: server.id,
          serverName: serverDisplayName(stores.serverInfo, server.name),
          viewerId: stores.projectionViewerId,
          rooms: ui.navigation.rooms,
          roomsLoading: ui.navigation.isInitialLoading,
          isUnread: (roomId) => ui.roomUnread.roomIsUnread(roomId),
          callRoomIds: ui.activeCallRooms.roomIds,
          callParticipants: (roomId) => ui.activeCallRooms.getParticipants(roomId)
        }
      ];
    })
  );
  const directMessages = $derived(homeDirectMessages(sources));
  const calls = $derived(homeLiveCalls(sources));
  const showServerName = $derived(sources.length > 1);
  const directMessagesLoading = $derived(homeDirectMessagesLoading(sources));
</script>

<PageTitle title={m('chat.home.title')} />

<div class="pane-page">
  <PaneHeader title={m('chat.home.title')} subtitle={m('chat.home.subtitle')} />

  <PaneContent wide>
    <!-- The side column comes first in both reading and visual order. -->
    <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
      <div class="flex min-w-0 flex-col gap-6">
        {#if calls.length > 0}
          <HomeLiveCalls {calls} {showServerName} />
        {/if}
        <HomeDirectMessages {directMessages} loading={directMessagesLoading} {showServerName} />
      </div>
      <div class="min-w-0">
        <NotificationFeed fillHeight={false} />
      </div>
    </div>
  </PaneContent>
</div>
