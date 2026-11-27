<!-- @component Choose a writable conversation before adding a device share to its draft. -->
<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { RoomKind, mapDirectoryRoomDetails } from '@chatto/client/api/roomDirectory';
  import { buildDirectMessagePresentation } from '@chatto/client/timeline/users';
  import { serverRegistry } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { directMessageLabels } from '$lib/render/directMessageLabels';
  import { isNavigationVisibleRoom } from '$lib/state/server/navigation';
  import {
    discardOtherShares,
    pendingShare,
    lastShareDestination,
    prepareShare,
    receiveShare,
    type IncomingShare
  } from '$lib/pwa/incomingShare';
  import { PageTitle } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';

  let share = $state.raw<IncomingShare | null | undefined>(undefined);
  const recent = lastShareDestination.get();
  let query = $state('');
  let opening = $state(false);
  let failed = $state(false);
  onMount(() => {
    const retry = pendingShare(page.state.incomingShare);
    if (
      retry &&
      serverRegistry.tryGetStore(retry.store.serverId) === retry.store &&
      retry.store.isAuthenticated
    ) {
      share = retry.share;
      return;
    }
    void receiveShare(location.hash.slice(1)).then(
      (value) => (share = value),
      () => (share = null)
    );
  });

  const options = $derived.by(() => {
    const servers = serverRegistry.servers;
    return servers.flatMap((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      const viewerId = store?.viewerId;
      if (!store?.isAuthenticated || !viewerId || !store.realtimeSync.hasUsableProjection)
        return [];
      const serverName = store.serverInfo.name || server.name || new URL(server.url).host;
      return store.roomList.rooms.flatMap((room) => {
        const details = mapDirectoryRoomDetails(store.projection.rooms.get(room.id));
        if (
          !room.viewerIsMember ||
          !isNavigationVisibleRoom(room) ||
          !details?.canPostMessage ||
          (share?.files.length && !details.canAttach)
        )
          return [];
        const dm = room.type === RoomKind.DM;
        if (
          dm &&
          room.members.length === 2 &&
          room.members.some(
            (member) => member.id !== viewerId && store.projection.users.isDeactivated(member.id)
          )
        )
          return [];
        const label = dm
          ? buildDirectMessagePresentation(room.members, viewerId, directMessageLabels()).label
          : room.name;
        return [
          {
            value: JSON.stringify([server.id, viewerId, room.id]),
            label,
            dm,
            serverName: servers.length > 1 ? serverName : '',
            store,
            roomId: room.id,
            path: conversationPath(server.id, room.id)
          }
        ];
      });
    });
  });
  const loadingRooms = $derived(
    serverRegistry.servers.some((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      return store?.isAuthenticated && store.roomList.isLoading;
    })
  );
  const matches = $derived.by(() => {
    const term = query.trim().replace(/^#/, '').toLocaleLowerCase();
    return options
      .filter((item) => (item.label + ' ' + item.serverName).toLocaleLowerCase().includes(term))
      .sort(
        (a, b) =>
          Number(b.value === recent) - Number(a.value === recent) || a.label.localeCompare(b.label)
      );
  });

  /** Each visible thumbnail owns its local URL until it leaves the picker. */
  function thumbnail(file: File) {
    return (image: HTMLImageElement) => {
      const url = URL.createObjectURL(file);
      image.src = url;
      return () => URL.revokeObjectURL(url);
    };
  }

  async function openDraft(value: string) {
    const selected = options.find((item) => item.value === value);
    if (!selected || !share || opening) return;
    opening = true;
    failed = false;
    const target = selected;
    const id = prepareShare(target.store, target.roomId, share, target.value);
    try {
      await goto(target.path, { replaceState: true, state: { incomingShare: id } });
    } catch {
      discardOtherShares();
      opening = false;
      failed = true;
    }
  }
</script>

<PageTitle title={m('chat.share_target.title')} />
<main class="mx-auto w-full max-w-lg px-4 pt-4 pb-8 sm:pt-12">
  <header class="mb-5 flex items-center gap-3">
    <img src="/icons/icon-192.png" alt="" class="size-9 shrink-0 rounded-lg" />
    <h1 class="flex-1 text-xl font-semibold text-text-top">{m('chat.share_target.title')}</h1>
    <Button
      size="icon"
      variant="ghost"
      label={m('common.cancel')}
      onclick={() => goto(resolve('/chat'), { replaceState: true })}
    >
      <span aria-hidden="true" class="iconify icon-[uil--times] size-5"></span>
    </Button>
  </header>
  {#if share === undefined}
    <p role="status">{m('common.loading')}</p>
  {:else if share === null}
    <p role="status">{m('chat.share_target.unavailable')}</p>
  {:else}
    <div class="preview-frame">
      <div class="flex flex-col gap-3 preview-content p-3">
        {#if share.files.length === 1}
          {@const file = share.files[0]}
          <div class="flex items-center gap-3">
            <img
              {@attach thumbnail(file)}
              alt=""
              class="h-20 w-24 shrink-0 preview-thumbnail object-cover"
            />
            <div class="min-w-0">
              {#if share.text}<p dir="auto" class="line-clamp-3 break-words whitespace-pre-wrap">
                  {share.text}
                </p>{/if}
              <p
                dir="auto"
                class="truncate"
                class:text-muted={!!share.text}
                class:text-sm={!!share.text}
              >
                {file.name}
              </p>
            </div>
          </div>
        {:else}
          {#if share.files.length}
            <div class="flex gap-2 overflow-x-auto">
              {#each share.files as file (file)}
                <figure class="w-20 shrink-0">
                  <img
                    {@attach thumbnail(file)}
                    alt=""
                    class="size-20 preview-thumbnail object-cover"
                  />
                  <figcaption dir="auto" class="truncate text-sm text-muted">
                    {file.name}
                  </figcaption>
                </figure>
              {/each}
            </div>
          {/if}
          {#if share.text}<p dir="auto" class="line-clamp-3 break-words whitespace-pre-wrap">
              {share.text}
            </p>{/if}
        {/if}
      </div>
    </div>
    <h2 class="mt-7 mb-2 text-lg font-semibold text-text-top">
      {m('chat.share_target.open_draft')}
    </h2>
    {#if options.length > 5 || query}
      <div class="mb-2">
        <TextInput
          id="share-search"
          label={m('room.directory.search_placeholder')}
          labelHidden
          leadingIcon="iconify icon-[uil--search]"
          placeholder={m('room.directory.search_placeholder')}
          bind:value={query}
        />
      </div>
    {/if}
    {#each matches as item (item.value)}
      <button
        type="button"
        disabled={opening}
        onclick={() => openDraft(item.value)}
        class="flex min-h-14 w-full cursor-pointer items-center gap-3 selectable-list-item rounded-lg px-2 text-start focus-visible:outline-2 focus-visible:outline-action"
      >
        <span
          aria-hidden="true"
          class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-surface text-muted"
        >
          {#if item.dm}
            <span aria-hidden="true" class="iconify icon-[uil--comment-alt-message] size-5"></span>
          {:else}
            <span class="text-xl font-medium">#</span>
          {/if}
        </span>
        <span class="min-w-0 flex-1">
          <span dir="auto" class="block truncate font-medium">{item.label}</span>
          {#if item.serverName}<span dir="auto" class="block truncate text-sm text-muted"
              >{item.serverName}</span
            >{/if}
        </span>
        {#if item.value === recent}<span class="text-sm text-muted"
            >{m('quick_switcher.recent')}</span
          >{/if}
        <span
          aria-hidden="true"
          class="iconify icon-[uil--angle-right] size-5 shrink-0 text-muted rtl:-scale-x-100"
        ></span>
      </button>
    {:else}
      <p role="status" class="text-muted">
        {query
          ? m('quick_switcher.no_results')
          : loadingRooms
            ? m('common.loading')
            : m('chat.share_target.no_rooms')}
      </p>
    {/each}
    {#if failed}<p role="alert">{m('common.error.generic')}</p>{/if}
  {/if}
</main>
