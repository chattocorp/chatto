<!-- @component Shows one call from a live notification or a notification link. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import { m } from '$lib/i18n/messages';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { getVoiceCallJoinErrorMessage } from '$lib/state/server/voiceCall.svelte';
  import { toast, Toast } from '$lib/ui/toast';

  let {
    store,
    roomId,
    callId,
    occurrenceId,
    fromLink,
    onclose
  }: {
    store: ServerStateStore;
    roomId: string;
    callId: string;
    occurrenceId?: string;
    fromLink: boolean;
    onclose: () => void;
  } = $props();

  const appUi = getAppUiState();
  const ui = $derived(serverUi(store));
  const ready = $derived(store.realtimeSync.phase === 'ready');
  const active = $derived(ui.activeCallRooms.getCallId(roomId) === callId);
  const joined = $derived(
    !!store.viewerId && !!ui.activeCallRooms.getParticipantCallPresence(roomId, store.viewerId)
  );
  const row = $derived(
    occurrenceId ? store.notifications.occurrences.find((item) => item.id === occurrenceId) : null
  );
  const roomName = $derived(ui.navigation.rooms.find((room) => room.id === roomId)?.name);
  const actor = $derived(row?.actor?.displayName || row?.actor?.login);
  const label = $derived(
    actor
      ? m('chat.notifications.summary.incoming_call', { actor })
      : m('voice.open_call_room', { room: roomName || store.serverInfo.name })
  );
  const permitted = $derived(ui.voiceCall.permissionsFor(roomId).join);
  // A delayed link may refer to a previous call in the same room. Wait for
  // the authenticated snapshot before deciding whether that call still exists.
  $effect(() => {
    if (!ready) return;
    if (!active || !permitted || joined || (!fromLink && row?.unread === false)) {
      untrack(() => {
        if (fromLink && !active) toast.info(m('room.system_events.call_ended'));
        onclose();
      });
    }
  });

  function dismiss() {
    onclose();
    if (row?.unread) void store.notifications.markOccurrenceRead(row.id).catch(() => {});
  }

  async function join() {
    const livekitUrl = store.serverInfo.livekitUrl;
    if (!ready || !active || !permitted || !livekitUrl || joined) return;
    const accountId = store.accountId;
    const current = () => !!accountId && store.accountId === accountId && store.isAuthenticated;
    try {
      appUi.requestRoomSidebarPanel(store.serverId, roomId, 'call', getRoomSidebarPresentation());
      await ui.voiceCall.join(livekitUrl, roomId, callId);
      if (current() && ui.voiceCall.matchesActiveCall(roomId, callId))
        await goto(conversationPath(store.serverId, roomId, { store }));
    } catch (error) {
      if (current()) toast.error(getVoiceCallJoinErrorMessage(error));
    }
  }
</script>

{#if ready && active && permitted && !joined}
  <div class="pointer-events-auto">
    <Toast
      tone="info"
      message={label}
      action={{ label: m('voice.join_call'), onClick: () => void join() }}
      onDismiss={dismiss}
    />
  </div>
{/if}
