<script lang="ts">
  import { useRoomUnread } from './useRoomUnread.svelte';

  let {
    roomId,
    onReady
  }: {
    roomId: string;
    onReady: (api: ReturnType<typeof useRoomUnread>) => void;
  } = $props();

  const unread = useRoomUnread(() => ({ roomId }));

  $effect(() => {
    onReady(unread);
  });
</script>
