<!--
@component

The server Overview: the server's identity, its welcome message and message of
the day, a setup reminder for administrators, current activity, the room
directory, and the server's Neighbors. Every section except the identity and
the directory hides itself when it has nothing to show.
-->
<script lang="ts">
  import { goto } from '$app/navigation';
  import ServerOverviewActivity, {
    type OverviewCall
  } from '$lib/components/overview/ServerOverviewActivity.svelte';
  import ServerOverviewHeader from '$lib/components/overview/ServerOverviewHeader.svelte';
  import ServerOverviewNeighbors from '$lib/components/overview/ServerOverviewNeighbors.svelte';
  import ServerOverviewNotes from '$lib/components/overview/ServerOverviewNotes.svelte';
  import ServerOverviewSetup from '$lib/components/overview/ServerOverviewSetup.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import { catchUpRooms, missingProfileItems } from '$lib/serverOverview';
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { PaneContent, PaneHeader, PageTitle } from '$lib/ui';

  // Re-derives reactively when the URL `[serverId]` changes. Directory rows
  // and membership are selected directly from that server's projection.
  const serverScope = useServerScope();
  const appUi = getAppUiState();

  const stores = serverScope.store;
  const ui = serverUi(stores);
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  /** Origin of the current server. It hosts the cached Neighbor profile images. */
  const serverOrigin = new URL(serverScope.connection.connectBaseUrl).origin;

  const profile = $derived(stores.projection.server);
  const missingProfile = $derived(
    profile && stores.permissions.canManageServer ? missingProfileItems(profile) : []
  );

  const calls = $derived.by((): OverviewCall[] => {
    const { navigation, activeCallRooms } = ui;
    return stores.projection.activeCalls.flatMap((call) => {
      const roomId = call.room?.id;
      if (!roomId || !navigation.isRoomMember(roomId)) return [];
      const participants = activeCallRooms.getParticipants(roomId);
      if (participants.length === 0) return [];
      return [{ roomId, roomName: call.room?.name ?? '', participants }];
    });
  });

  const catchUp = $derived.by(() => {
    const { navigation, roomUnread } = ui;
    return catchUpRooms(navigation.rooms, (roomId) => roomUnread.roomIsUnread(roomId));
  });

  /** Open the room with its call panel, where the viewer joins the call. */
  function openCall(roomId: string) {
    appUi.requestRoomSidebarPanel(
      serverScope.serverId,
      roomId,
      'call',
      getRoomSidebarPresentation()
    );
    void goto(conversationPath(serverScope.serverId, roomId, { store: stores }));
  }
</script>

<PageTitle title={m('chat.overview.title')} />

<div class="pane-page">
  <PaneHeader title={m('chat.overview.title')} />

  <!-- The directory's room groups are titled panels; the masonry uses the wide width. -->
  <PaneContent wide>
    <div class="flex flex-col gap-8">
      {#if profile}
        <ServerOverviewHeader
          name={profile.name}
          logoUrl={profile.logoUrl}
          bannerUrl={profile.bannerUrl}
          description={profile.description}
        />
      {/if}

      <ServerOverviewSetup
        serverId={serverScope.serverId}
        {serverSegment}
        missing={missingProfile}
      />

      <ServerOverviewNotes
        serverId={serverScope.serverId}
        welcomeMessage={profile?.welcomeMessage}
        motd={stores.projection.serverState?.motd}
      />

      <ServerOverviewActivity {serverSegment} {calls} {catchUp} onjoincall={openCall} />

      <section aria-label={m('common.rooms')}>
        <RoomDirectory directory={ui.roomDirectory} {serverSegment} />
      </section>

      <ServerOverviewNeighbors {serverOrigin} />
    </div>
  </PaneContent>
</div>
