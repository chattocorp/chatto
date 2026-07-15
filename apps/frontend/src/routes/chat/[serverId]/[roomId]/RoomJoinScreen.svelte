<script lang="ts">
  import { resolve } from '$app/paths';
  import * as m from '$lib/i18n/messages';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
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
  const groupName = $derived(
    stores.rooms.roomGroups?.find((group) => group.roomIds.includes(room.id))?.name ?? null
  );
  const description = $derived(room.description?.trim() || null);

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

<section
  class="flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-y-auto bg-background px-4 py-8 sm:px-6 sm:py-10"
>
  {#if room.viewerCanJoinRoom}
    <div
      class="flex w-full max-w-lg flex-col items-center panel-shell p-6 text-center panel-shell-raised sm:p-8"
      data-testid="room-join-preview"
    >
      <h1 class="text-2xl font-semibold text-text-top">
        {title}
      </h1>

      {#if groupName}
        <p class="mt-1 flex items-center justify-center gap-1.5 text-sm text-muted">
          <span class="iconify uil--folder" aria-hidden="true"></span>
          {m['room.join.group_label']({ group: groupName })}
        </p>
      {/if}

      {#if description}
        <p class="mt-4 text-base leading-7 text-pretty text-text">{description}</p>
      {/if}

      {#await stores.roomDirectory.loadJoinPreview(room.id)}
        <div
          class="mt-6 flex min-h-20 w-full flex-col items-center justify-center surface-box rounded-lg px-4 py-4"
          aria-label={m['room.join.member_preview_label']()}
        >
          <div class="flex flex-col items-center gap-3" aria-hidden="true">
            <div class="skeleton h-4 w-24 rounded"></div>
            <div class="flex -space-x-2">
              <div class="skeleton h-8 w-8 rounded-full ring-2 ring-surface"></div>
              <div class="skeleton h-8 w-8 rounded-full ring-2 ring-surface"></div>
              <div class="skeleton h-8 w-8 rounded-full ring-2 ring-surface"></div>
            </div>
          </div>
        </div>
      {:then preview}
        {#if preview}
          <div
            class="mt-6 flex min-h-20 w-full flex-col items-center justify-center surface-box rounded-lg px-4 py-4"
            aria-label={m['room.join.member_preview_label']()}
          >
            <h2 class="font-medium text-text">
              {m['room.join.member_count']({ count: preview.memberCount })}
            </h2>
            {#if preview.sampleMembers.length > 0}
              <div class="mt-3 flex shrink-0 -space-x-2" aria-hidden="true">
                {#each preview.sampleMembers as member (member.id)}
                  <UserAvatar user={member} size="sm" class="ring-2 ring-surface" />
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/await}

      <p class="mt-6 text-base leading-7 text-muted">
        {m['room.join.inline_prompt']()}
      </p>

      <div class="mt-6 flex flex-wrap justify-center gap-2">
        <Button loading={joining} onclick={() => void joinRoom()}>
          <span class="iconify uil--plus"></span>
          {m['room.join.action']()}
        </Button>
      </div>
    </div>
  {:else}
    <div class="flex w-full max-w-md flex-col items-center text-center">
      <div
        class="mb-5 flex h-12 w-12 items-center justify-center rounded-full border border-border bg-surface text-muted"
        aria-hidden="true"
      >
        <span class="iconify text-2xl uil--lock"></span>
      </div>

      <h1 class="text-2xl font-semibold text-text">
        {title}
      </h1>
      <p class="mt-3 text-base leading-7 text-muted">
        {m['room.join.access_denied']()}
      </p>

      <div class="mt-6 flex flex-wrap justify-center gap-2">
        <Button href={overviewPath} variant="secondary">
          <span class="iconify uil--arrow-left"></span>
          {m['ui.access_denied.back_to_server']()}
        </Button>
      </div>
    </div>
  {/if}
</section>
