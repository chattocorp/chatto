<!--
@component

Loads one pending highlight while mounted. The parent keys this component by
request identity and mounts it only while snapshot recovery is complete.
Unmounting cancels completion reporting without removing the owner's request.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { PendingHighlight } from '$lib/state/server/pendingHighlight';

  let {
    request,
    jump,
    onFailed
  }: {
    request: PendingHighlight;
    jump: (eventId: string) => Promise<boolean>;
    onFailed: (request: PendingHighlight) => void;
  } = $props();

  onMount(() => {
    const target = request;
    let cancelled = false;
    void jump(target.eventId).then((loaded) => {
      if (!cancelled && !loaded) onFailed(target);
    });
    return () => {
      cancelled = true;
    };
  });
</script>
