<!-- @component Shows one call from a live notification. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import { m } from '$lib/i18n/messages';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { serverUi } from '$lib/state/server/serverUi';
  import { Toast } from '$lib/ui/toast';

  let {
    store,
    roomId,
    callId,
    occurrenceId,
    onclose
  }: {
    store: ServerStateStore;
    roomId: string;
    callId: string;
    occurrenceId: string;
    onclose: () => void;
  } = $props();

  const ui = $derived(serverUi(store));
  const ready = $derived(store.realtimeSync.phase === 'ready');
  const active = $derived(ui.activeCallRooms.getCallId(roomId) === callId);
  const joined = $derived(
    !!store.viewerId && !!ui.activeCallRooms.getParticipantCallPresence(roomId, store.viewerId)
  );
  const row = $derived(store.notifications.occurrences.find((item) => item.id === occurrenceId));
  const roomName = $derived(ui.navigation.rooms.find((room) => room.id === roomId)?.name);
  const actor = $derived(row?.actor?.displayName || row?.actor?.login);
  const label = $derived(
    actor
      ? m('chat.notifications.summary.incoming_call', { actor })
      : m('voice.open_call_room', { room: roomName || store.serverInfo.name })
  );
  const permitted = $derived(ui.voiceCall.permissionsFor(roomId).join);
  $effect(() => {
    if (ready && (!active || !permitted || joined || row?.unread === false)) untrack(onclose);
  });

  function dismiss() {
    onclose();
    if (row?.unread) void store.notifications.markOccurrenceRead(row.id).catch(() => {});
  }

  function openRoom() {
    void goto(conversationPath(store.serverId, roomId, { store }));
  }
</script>

{#if ready && active && permitted && !joined}
  <div class="pointer-events-auto">
    <Toast
      tone="info"
      message={label}
      action={{ label: m('voice.open_room'), onClick: openRoom }}
      onDismiss={dismiss}
    />
  </div>
{/if}
