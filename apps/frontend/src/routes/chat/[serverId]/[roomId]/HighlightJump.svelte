<!--
@component

Loads one pending highlight while mounted. The parent keys this component by
request identity and mounts it only while snapshot recovery is complete.
Unmounting cancels completion reporting without removing the owner's request.
-->
<script lang="ts">
  import { onMount, tick } from 'svelte';
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
    void (async () => {
      // The parent thread must activate its window before this load starts.
      await tick();
      if (cancelled) return;
      const loaded = await jump(target.eventId);
      if (!cancelled && !loaded) onFailed(target);
    })();
    return () => {
      cancelled = true;
    };
  });
</script>
