<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';

  let {
    onclose
  }: {
    onclose: () => void;
  } = $props();

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  function handleRoomCreated(roomId: string) {
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }
</script>

<Dialog visible title={m['room.create.title']()} size="md" {onclose}>
  <p class="mb-4 text-muted">{m['room.create.description']()}</p>
  <CreateRoom onroomcreated={handleRoomCreated} />
</Dialog>
