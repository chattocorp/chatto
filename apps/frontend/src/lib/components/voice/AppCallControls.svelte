<!-- @component Call controls for the connected call, across all registered servers. -->
<script lang="ts">
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { goto } from '$app/navigation';
  import { serverUi } from '$lib/state/server/serverUi';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
  import { buildDirectMessagePresentation } from '@chatto/client/timeline/users';
  import { directMessageLabels } from '$lib/render/directMessageLabels';
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { m } from '$lib/i18n/messages';
  import VoiceCallMediaControls from './VoiceCallMediaControls.svelte';
  import VoiceCallControlButton from './VoiceCallControlButton.svelte';
  import { PillButtonGroup } from '$lib/ui';

  let { activeStore }: { activeStore: ServerStateStore } = $props();
  const appUi = getAppUiState();
  const activeServerId = $derived(activeStore.serverId);
  const voiceCallState = $derived(serverUi(activeStore).voiceCall);
  const navigation = $derived(serverUi(activeStore).navigation);
  const activeCallRoomId = $derived(
    voiceCallState?.connected && voiceCallState.roomId ? voiceCallState.roomId : null
  );
  const activeCallRoom = $derived(
    activeCallRoomId
      ? (navigation?.rooms.find((room) => room.id === activeCallRoomId) ?? null)
      : null
  );
  const activeCallRoomName = $derived.by(() => {
    const room = activeCallRoom;
    if (!room) return m('common.current_call');
    if (room.type === RoomKind.DM) {
      return buildDirectMessagePresentation(
        room.members,
        activeStore.projectionViewerId,
        directMessageLabels(),
        (userId, fallback) => {
          const users = activeStore.projection.users;
          return users.isDeleted(userId) ? '' : (users.view(userId)?.displayName ?? fallback);
        }
      ).label;
    }
    return `# ${room.name}`;
  });

  function openActiveCallRoom(): void {
    const roomId = activeCallRoomId;
    if (!roomId) return;

    appUi.requestRoomSidebarPanel(activeServerId, roomId, 'call', getRoomSidebarPresentation());
    goto(conversationPath(activeServerId, roomId, { store: activeStore }));
  }
</script>

{#if activeCallRoomId && voiceCallState}
  <div class="app-call-frame">
    <PillButtonGroup
      class="app-call-bar"
      label={m('room.sidebar.call')}
      testId="app-header-call-card"
    >
      <VoiceCallControlButton
        class="pill-button"
        label={m('voice.open_call_room', { room: activeCallRoomName })}
        testId="app-header-call-link"
        icon="icon-[uil--phone]"
        iconClass="text-lg text-action"
        onclick={openActiveCallRoom}
      />
      <VoiceCallMediaControls
        {voiceCallState}
        testIds={{
          mute: 'app-header-call-mute',
          camera: 'app-header-call-camera',
          screenShare: 'app-header-call-screen-share',
          leave: 'app-header-call-leave'
        }}
      />
    </PillButtonGroup>
  </div>
{/if}
