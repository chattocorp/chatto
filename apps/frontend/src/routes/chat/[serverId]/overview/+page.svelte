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
  import ServerOverviewDirectMessages from '$lib/components/overview/ServerOverviewDirectMessages.svelte';
  import ServerOverviewHeader from '$lib/components/overview/ServerOverviewHeader.svelte';
  import ServerOverviewNeighbors from '$lib/components/overview/ServerOverviewNeighbors.svelte';
  import ServerOverviewSetup from '$lib/components/overview/ServerOverviewSetup.svelte';
  import ServerOverviewWelcome from '$lib/components/overview/ServerOverviewWelcome.svelte';
  import MarkdownContent from '$lib/components/MarkdownContent.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { openRoomNotification } from '$lib/notifications/openRoomNotification';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import { catchUpRooms, missingProfileItems, recentDirectMessages } from '$lib/serverOverview';
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { PaneContent, PaneHeader, PageTitle, Panel } from '$lib/ui';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import type { NotificationAttentionLevel } from '@chatto/client/api/notifications';

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

  const directMessages = $derived(recentDirectMessages(ui.navigation.rooms));

  const motd = $derived(stores.projection.serverState?.motd?.trim() || null);
  /** The side column shows only when it has content; otherwise the main column takes the width. */
  const hasAside = $derived(
    !!motd || calls.length > 0 || catchUp.length > 0 || directMessages.length > 0
  );

  function openNotification(roomId: string, attentionLevel: NotificationAttentionLevel) {
    const isDM = ui.navigation.rooms.some(
      (room) => room.id === roomId && room.type === RoomKind.DM
    );
    void openRoomNotification(serverScope, appUi, roomId, { isDM, attentionLevel });
  }

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
    <div class="@container flex flex-col gap-8">
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

      <!--
        Wide containers: the welcome message and rooms on the left, the side column
        on the right. The side column spans both rows, and the second row takes
        the extra height, so a tall side column does not open a gap below the
        welcome message. Narrow containers stack welcome, side column, and rooms.
      -->
      <div
        class={[
          'grid gap-8',
          hasAside && '@4xl:grid-cols-[minmax(0,1fr)_22rem] @4xl:grid-rows-[auto_1fr]'
        ]}
      >
        <div class="min-w-0 empty:hidden @4xl:col-start-1 @4xl:row-start-1">
          <ServerOverviewWelcome
            serverId={serverScope.serverId}
            welcomeMessage={profile?.welcomeMessage}
          />
        </div>

        {#if hasAside}
          <aside
            class="flex min-w-0 flex-col gap-6 @4xl:col-start-2 @4xl:row-span-2 @4xl:row-start-1"
          >
            {#if motd}
              <Panel title={m('common.message_of_the_day')} icon="iconify icon-[uil--megaphone]">
                <MarkdownContent source={motd} />
              </Panel>
            {/if}
            <ServerOverviewActivity
              {serverSegment}
              {calls}
              {catchUp}
              onjoincall={openCall}
              onopennotification={openNotification}
            />
            <ServerOverviewDirectMessages
              conversations={directMessages}
              viewerId={stores.projectionViewerId}
              serverId={serverScope.serverId}
              isUnread={(roomId) => ui.roomUnread.roomIsUnread(roomId)}
              getDisplayName={getLiveDisplayName}
              onopennotification={openNotification}
            />
          </aside>
        {/if}

        <section class="min-w-0 @4xl:col-start-1 @4xl:row-start-2" aria-label={m('common.rooms')}>
          <RoomDirectory directory={ui.roomDirectory} {serverSegment} />
        </section>
      </div>

      <ServerOverviewNeighbors {serverOrigin} />
    </div>
  </PaneContent>
</div>
