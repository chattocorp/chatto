<script lang="ts">
  import { resolve } from '$app/paths';
  import * as m from '$lib/i18n/messages';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import type { RoomsListItem } from '$lib/state/server/rooms.svelte';

  let {
    room,
    serverSegment
  }: {
    room: RoomsListItem;
    serverSegment: string;
  } = $props();

  const activeServerId = $derived(getActiveServer());
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const overviewPath = $derived(resolve('/chat/[serverId]', { serverId: serverSegment }));
  const title = $derived(`#${room.name}`);
  let joining = $state(false);

  async function joinRoom(): Promise<void> {
    if (joining || !room.viewerCanJoinRoom) return;

    joining = true;
    try {
      const result = await stores.roomDirectory.joinRoom(room.id);

      if (!result.ok) {
        toast.error(m['room.join.failed']());
        console.error('Error joining room:', result.error);
        return;
      }

      toast.success(
        result.room
          ? m['room.join.success']({ room: result.room.name })
          : m['room.join.success_generic']()
      );
      await stores.rooms.refresh();
    } finally {
      joining = false;
    }
  }
</script>

<PageTitle {title} />

<section class="flex min-h-0 min-w-0 flex-1 items-center justify-center bg-background px-6 py-10">
  <div class="flex w-full max-w-md flex-col items-center text-center">
    <div
      class={[
        'mb-5 flex h-12 w-12 items-center justify-center rounded-full border',
        room.viewerCanJoinRoom
          ? 'border-neutral-action/30 bg-neutral-action/10 text-neutral-action'
          : 'border-border bg-surface text-muted'
      ]}
      aria-hidden="true"
    >
      {#if room.viewerCanJoinRoom}
        <span class="iconify text-2xl uil--plus"></span>
      {:else}
        <span class="iconify text-2xl uil--lock"></span>
      {/if}
    </div>

    <h1 class="text-2xl font-semibold text-text">
      {title}
    </h1>
    <p class="mt-3 text-base leading-7 text-muted">
      {#if room.viewerCanJoinRoom}
        {m['room.join.inline_prompt']()}
      {:else}
        {m['room.join.access_denied']()}
      {/if}
    </p>

    <div class="mt-6 flex flex-wrap justify-center gap-2">
      {#if room.viewerCanJoinRoom}
        <Button loading={joining} onclick={() => void joinRoom()}>
          <span class="iconify uil--plus"></span>
          {m['room.join.action']()}
        </Button>
      {:else}
        <Button href={overviewPath} variant="secondary">
          <span class="iconify uil--arrow-left"></span>
          {m['ui.access_denied.back_to_server']()}
        </Button>
      {/if}
    </div>
  </div>
</section>
