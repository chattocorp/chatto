<!--
@component

Activates the projected room window while mounted. The parent keys this
component by room ID and mounts it only when room hydration is active.
An unfinished root jump owns its historical window through hydration changes
and remounts. Departure cancels only the old room's request.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import type { RoomNavigationState } from './roomNavigationState.svelte';

  let {
    roomId,
    store,
    navigation,
    isActive
  }: {
    roomId: string;
    store: ServerStateStore;
    navigation: RoomNavigationState;
    isActive: (roomId: string) => boolean;
  } = $props();

  onMount(() => {
    const selectedRoomId = roomId;
    const restoreLatest = () => {
      if (!navigation.highlightFor(selectedRoomId, null)) {
        store.restoreProjectedRoomWindow(selectedRoomId);
      }
    };
    restoreLatest();
    return () => {
      if (!isActive(selectedRoomId)) navigation.clearMainHighlight(selectedRoomId);
      restoreLatest();
    };
  });
</script>
