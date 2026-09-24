<script lang="ts">
  import { onMount } from 'svelte';
  import type { Activity } from '$lib/timeline.ts';
  import ActivityDetails from './ActivityDetails.svelte';

  let { activity, elapsed, onclose }: { activity: Activity; elapsed: number; onclose: () => void } =
    $props();
  let dialog: HTMLDialogElement;
  const titleId = $props.id();

  onMount(() => {
    dialog.showModal();
  });
</script>

<dialog class="modal modal-middle" bind:this={dialog} {onclose} aria-labelledby={titleId}>
  <div class="modal-box flex max-h-[85dvh] w-11/12 max-w-4xl flex-col overflow-hidden p-0">
    <ActivityDetails {activity} {elapsed} {titleId} onclose={() => dialog.close()} />
  </div>
  <form method="dialog" class="modal-backdrop">
    <button class="cursor-pointer" aria-label="Close activity details">Close</button>
  </form>
</dialog>
