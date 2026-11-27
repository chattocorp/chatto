<!-- @component
Call controls for the connected call, across all registered servers. The room
button names the call, glows while the user speaks, and shows a poor or lost
connection. With more than one registered server, the server logo comes first.
It opens the call room.
-->
<script lang="ts">
  import { bindDesktopMuteShortcut, bindDesktopPushToTalk } from '$lib/desktop/muteShortcut';
  import { shortcutPreferences } from '$lib/state/shortcutPreferences.svelte';
  import { toastError } from '$lib/utils/errorMessage';
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
  import { PillButtonGroup, VoiceActivity } from '$lib/ui';
  import { voiceActivityLevel } from '$lib/state/server/voiceCall.svelte';
  import { serverRegistry } from '$lib/client';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { roomIcon } from '$lib/render/roomIcon';
  import ServerLogo from '$lib/components/ServerLogo.svelte';

  let { activeStore }: { activeStore: ServerStateStore } = $props();
  const appUi = getAppUiState();
  const activeServerId = $derived(activeStore.serverId);
  const voiceCallState = $derived(serverUi(activeStore).voiceCall);
  $effect(() => {
    if (!voiceCallState.connected || !voiceCallState.canUseVoice || shortcutPreferences.recording)
      return;
    return bindDesktopMuteShortcut(
      shortcutPreferences.bindings.mute,
      () => void voiceCallState.toggleMute().catch((error) => toastError(error))
    );
  });
  $effect(() => {
    const call = voiceCallState;
    if (!call.connected || !call.canUseVoice || shortcutPreferences.recording) return;
    return bindDesktopPushToTalk(
      shortcutPreferences.bindings.pushToTalk,
      (held) => void call.setPushToTalkHeld(held).catch((error) => toastError(error))
    );
  });
  const navigation = $derived(serverUi(activeStore).navigation);
  const activeCallRoomId = $derived(
    voiceCallState?.connected && voiceCallState.roomId ? voiceCallState.roomId : null
  );
  const activeCallRoom = $derived(
    activeCallRoomId
      ? (navigation?.rooms.find((room) => room.id === activeCallRoomId) ?? null)
      : null
  );
  /** Room or DM name without a room-type prefix, or null before navigation knows the room. */
  const activeCallRoomTitle = $derived.by(() => {
    const room = activeCallRoom;
    if (!room) return null;
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
    return room.name;
  });
  const activeCallRoomIcon = $derived(activeCallRoom ? roomIcon(activeCallRoom) : null);
  /** Name for labels and tooltips, with the `#` of a room. */
  const activeCallRoomName = $derived.by(() => {
    if (activeCallRoomTitle === null) return m('common.current_call');
    return activeCallRoomIcon?.text
      ? `${activeCallRoomIcon.text} ${activeCallRoomTitle}`
      : activeCallRoomTitle;
  });
  // Several registered servers can have rooms with the same name.
  const showServer = $derived(serverRegistry.servers.length > 1);
  const callServer = $derived.by(() => {
    const registered = serverRegistry.getServer(activeServerId);
    return {
      name: serverDisplayName(activeStore.serverInfo, registered?.name),
      logoUrl: activeStore.serverInfo.iconUrl ?? registered?.iconUrl
    };
  });
  const openLabel = $derived(
    showServer
      ? m('voice.open_call_room_on_server', { room: activeCallRoomName, server: callServer.name })
      : m('voice.open_call_room', { room: activeCallRoomName })
  );

  const localParticipant = $derived(voiceCallState.participants.find((p) => p.isLocal));
  const connectionWarning = $derived.by(() => {
    const quality = localParticipant?.connectionQuality;
    if (quality === 'lost') return { label: m('voice.connection_lost'), lost: true };
    if (quality === 'poor') return { label: m('voice.poor_connection'), lost: false };
    return null;
  });
  const warningId = $props.id();

  /** Own voice level for the room button's glow; read each animation frame. */
  function ownVoiceLevel(): number {
    if (!localParticipant || voiceCallState.isMuted) return 0;
    return voiceActivityLevel(voiceCallState.getAudioLevel(localParticipant.identity));
  }

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
      <button
        type="button"
        class="pill-button isolate"
        data-call-room
        title={openLabel}
        aria-label={openLabel}
        aria-describedby={connectionWarning ? warningId : undefined}
        data-testid="app-header-call-link"
        onclick={openActiveCallRoom}
      >
        <VoiceActivity level={ownVoiceLevel} />
        {#if connectionWarning}
          <span
            class={[
              'iconify shrink-0 text-base',
              connectionWarning.lost
                ? 'icon-[mdi--wifi-off] text-danger'
                : 'icon-[mdi--wifi-strength-1] text-warning'
            ]}
            aria-hidden="true"
          ></span>
          <span id={warningId} class="sr-only">{connectionWarning.label}</span>
        {:else}
          <span class="size-2 shrink-0 rounded-full bg-success" aria-hidden="true"></span>
        {/if}
        <span class="flex min-w-0 items-center gap-1.5 max-sm:hidden">
          {#if showServer}
            <span class="size-4 shrink-0 overflow-hidden rounded-[4px]" aria-hidden="true">
              <ServerLogo server={callServer} compact />
            </span>
          {/if}
          <span class="flex min-w-0 items-center gap-1">
            {#if activeCallRoomTitle !== null && activeCallRoomIcon}
              <span class={['shrink-0 text-muted', activeCallRoomIcon.className]} aria-hidden="true"
                >{activeCallRoomIcon.text}</span
              >
            {/if}
            <bdi class="min-w-0 truncate text-text">{activeCallRoomTitle ?? activeCallRoomName}</bdi
            >
          </span>
        </span>
      </button>
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
