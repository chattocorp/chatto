<script lang="ts">
  import { flushSync } from 'svelte';
  import { commit, get_native, read_native, type ShadowNode } from 'gpuix-svelte';
  import type { Server } from '@chatto/client';
  import { RoomKind } from '@chatto/client';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import ActionButton from './ActionButton.svelte';
  import Avatar from './ui/Avatar.svelte';
  import Icon from './ui/Icon.svelte';
  import ChannelRow from './ui/ChannelRow.svelte';
  import SidebarSection from './ui/SidebarSection.svelte';
  import Composer from './ui/Composer.svelte';
  import Gallery from './Gallery.svelte';
  import { inputTheme } from './ui/theme.ts';
  import { messageDate, messageDay } from './ui/messageDates.ts';
  import LiveMessage from './LiveMessage.svelte';
  import type { NativeSession } from './session.svelte.ts';

  let { session, server }: { session: NativeSession; server: Server } = $props();
  const rooms = $derived(server.roomList.rooms);
  const groups = $derived(server.roomList.roomGroups);
  const otherRooms = $derived(
    rooms.filter(
      (room) =>
        room.type === RoomKind.DM || !groups.some((group) => group.roomIds.includes(room.id))
    )
  );
  const selectedRoom = $derived(rooms.find((room) => room.id === session.selected));
  const timeline = $derived(session.selected ? server.rooms.messages(session.selected) : null);
  const events = $derived(
    timeline?.events.filter(
      (event) => isMessagePostedEvent(event.event) && !event.event.deletedAt
    ) ?? []
  );
  const viewer = $derived(server.viewerUser);
  let draft = $state('');
  let sending = $state(false);
  let sendError = $state('');
  let historyNotice = $state('');
  let search = $state('');
  let gallery = $state(false);
  const matchingRooms = $derived(
    rooms.filter((room) => roomLabel(room).toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  );
  const matchingOtherRooms = $derived(
    otherRooms.filter((room) => matchingRooms.some((match) => match.id === room.id))
  );
  let listNode: ShadowNode | null = null;
  // Virtualization does not unmount Svelte rows. Fence image reads and decoded
  // data to the visible range so long histories do not fetch every attachment.
  let visible = $state({ start: -1, end: -1 });

  function roomLabel(room: RoomListItem): string {
    if (room.type !== RoomKind.DM) return room.name;
    const others = room.members.filter((member) => member.id !== viewer?.id);
    return (
      others
        .map((member) => (member.deleted ? 'Deleted user' : member.displayName || member.login))
        .join(', ') || `${viewer?.displayName || viewer?.login || 'You'} (You)`
    );
  }
  function choose(id: string) {
    draft = '';
    sendError = '';
    historyNotice = '';
    session.select(id);
  }
  async function older(start: number) {
    if (
      start > 2 ||
      !timeline ||
      timeline.isInitialLoading ||
      timeline.isLoadingMore ||
      timeline.hasReachedStart
    )
      return;
    const owner = timeline;
    const node = listNode;
    const generation = session.draftGeneration;
    const previousCount = owner.events.length;
    const previousFirst = owner.events[0]?.id;
    historyNotice = '';
    const offset =
      node?.nativeId == null
        ? null
        : read_native((native) => native.getListScrollTop?.(node.nativeId!));
    const anchor = offset ? events[offset[0] ?? 0]?.id : undefined;
    await owner.loadMore();
    flushSync();
    if (timeline !== owner || session.draftGeneration !== generation) return;
    // The SDK logs page failures in the host terminal rather than rejecting.
    // An empty advancing page is also possible, so report only lack of rows.
    if (
      !owner.hasReachedStart &&
      owner.events.length === previousCount &&
      owner.events[0]?.id === previousFirst
    ) {
      historyNotice =
        'No additional messages were loaded. Try Load older messages again; if this persists, check the terminal output.';
    }
    if (
      !anchor ||
      !offset ||
      !node ||
      listNode !== node ||
      timeline !== owner ||
      session.draftGeneration !== generation
    )
      return;
    const index = events.findIndex((event) => event.id === anchor);
    if (index < 0 || node.nativeId == null) return;
    // Commit the prepended rows first. Native applies this anchor after its
    // child splice, so a newly loaded page cannot move the visible message.
    commit();
    if (node.nativeId != null) get_native()?.scrollToItem?.(node.nativeId, index, offset[1] ?? 0);
  }
  async function send() {
    if (!draft.trim() || sending || !session.selected || !timeline?.isCurrent) return;
    const body = draft;
    const room = session.selected;
    const version = session.draftGeneration;
    sending = true;
    sendError = '';
    try {
      await server.createMessage({ roomId: room, threadRootId: '' }, body);
      if (session.selected === room && session.draftGeneration === version) draft = '';
    } catch {
      sendError = 'The message could not be confirmed. Check the conversation before trying again.';
    } finally {
      sending = false;
    }
  }
</script>

{#if gallery}
  <Gallery onClose={() => (gallery = false)} />
{:else}
  <div class="connected">
    <div class="server-rail">
      <div class="current-server">
        <Avatar name={server.projection.server?.name || 'Chatto'} size={36} />
      </div>
    </div>
    <div class="sidebar">
      <div class="brand">
        <text class="server-name">{server.projection.server?.name || 'Chatto'}</text>
      </div>
      <div class="search">
        <Icon name="search" size={16} /><input
          class="search-input"
          value={search}
          theme={{ ...inputTheme, bg: '#171717' }}
          placeholder="Search conversations"
          aria-label="Search conversations"
          onchange={(event) => (search = event.value)}
          testid="room-search"
        />
      </div>
      <div class="rooms">
        {#each groups as group (group.id)}
          {#if group.roomIds.some( (id) => matchingRooms.some((room) => room.id === id && room.type !== RoomKind.DM) )}
            <SidebarSection label={group.name.toUpperCase()}>
              {#each group.roomIds as id (id)}
                {@const room = matchingRooms.find((room) => room.id === id)}
                {#if room && room.type !== RoomKind.DM}
                  <ChannelRow
                    label={roomLabel(room)}
                    selected={session.selected === room.id}
                    unread={server.notifications.roomImportantUnreadCounts[room.id] || 0}
                    testid={`room-${id}`}
                    onclick={() => choose(id)}
                  />
                {/if}
              {/each}
            </SidebarSection>
          {/if}
        {/each}
        {#if matchingOtherRooms.length}
          <SidebarSection label="CONVERSATIONS">
            {#each matchingOtherRooms as room (room.id)}
              <ChannelRow
                label={roomLabel(room)}
                direct={room.type === RoomKind.DM}
                selected={session.selected === room.id}
                unread={server.notifications.roomImportantUnreadCounts[room.id] || 0}
                testid={`room-${room.id}`}
                onclick={() => choose(room.id)}
              />
            {/each}
          </SidebarSection>
        {/if}
        {#if search && !matchingRooms.length}<text class="empty-search">No conversations found</text
          >{/if}
      </div>
      <div class="account">
        <Avatar
          name={viewer?.displayName || viewer?.login || '?'}
          src={viewer?.avatarUrl}
          serverUrl={server.serverUrl}
          size={30}
        />
        <div class="identity">
          <text class="account-name"
            >{viewer?.displayName || viewer?.login || 'Loading account…'}</text
          ><text class="muted">{viewer?.login ? `@${viewer.login}` : server.connection.status}</text
          >
        </div>
        <ActionButton
          label="Component gallery"
          icon="gallery"
          iconOnly
          quiet
          onclick={() => (gallery = true)}
        />
        <ActionButton
          label="Sign out"
          icon="logout"
          iconOnly
          quiet
          onclick={() => void session.signOut()}
        />
      </div>
    </div>
    <div class="conversation">
      <div class="header">
        <Icon name={selectedRoom?.type === RoomKind.DM ? 'message' : 'hash'} size={20} />
        <text class="title">{selectedRoom ? roomLabel(selectedRoom) : 'Select a conversation'}</text
        >
        {#if selectedRoom?.description}<text class="topic">{selectedRoom.description}</text>{/if}
        <div class="header-space"></div>
        <text class="connection">{server.connection.status}</text>
      </div>
      {#if session.error}<text class="error">{session.error}</text>{/if}
      {#if timeline?.loadError}<text class="error">Could not load this conversation.</text
        ><ActionButton label="Retry" onclick={() => void timeline?.restoreLatestWindow()} />{/if}
      {#if timeline?.isInitialLoading || timeline?.isLoadingMore}<text class="muted status"
          >Loading messages…</text
        >{/if}
      {#if timeline && !timeline.isInitialLoading}
        <div class="history-status">
          <text class="muted"
            >{events.length} messages loaded · {timeline.hasReachedStart
              ? 'Beginning of available history'
              : 'More history available'}</text
          >
          {#if !timeline.hasReachedStart}
            <ActionButton
              label={timeline.isLoadingMore ? 'Loading older messages…' : 'Load older messages'}
              disabled={timeline.isLoadingMore}
              quiet
              testid="load-older"
              onclick={() =>
                void older(0).catch(() => {
                  session.error = 'Older history could not be loaded';
                })}
            />
          {/if}
        </div>
      {/if}
      {#if historyNotice}<text class="muted status">{historyNotice}</text>{/if}
      {#key `${server.serverId}:${session.selected}:${session.draftGeneration}`}
        <virtual-list
          class="timeline"
          alignment="bottom"
          followTail={true}
          overdraw={240}
          estimatedItemHeight={100}
          onvisiblerange={(event) => {
            visible = {
              start: event.startIndex ?? 0,
              end: event.endIndex ?? event.startIndex ?? 0
            };
            void older(event.startIndex ?? 0).catch(() => {
              session.error = 'Older history could not be loaded';
            });
          }}
          testId="live-timeline"
          {@attach (node) => {
            listNode = node;
            visible = { start: -1, end: -1 };
            // Native does not report every initial range or follow-tail change.
            // Like GPUix's Scroller, sample its cheap anchor read without asking
            // for bounds (bounds reads can draw a frame on other platforms).
            const timer = setInterval(() => {
              if (listNode !== node || node.nativeId == null) return;
              const top = read_native((native) => native.getListScrollTop(node.nativeId!));
              if (!top) return;
              const start = top[0] ?? 0;
              // Every message row is at least 50px tall. This conservative
              // window covers visible rows without loading the whole history.
              const end = start + Math.ceil((top[2] ?? 500) / 50);
              const moved = start !== visible.start;
              if (moved || end !== visible.end) visible = { start, end };
              if (moved)
                void older(start).catch(() => {
                  session.error = 'Older history could not be loaded';
                });
            }, 200);
            return () => {
              clearInterval(timer);
              if (listNode === node) listNode = null;
            };
          }}
        >
          {#each events as item, index (item.id)}
            <LiveMessage
              {item}
              {server}
              dayLabel={index === 0 ||
              messageDay(events[index - 1]!.createdAt) !== messageDay(item.createdAt)
                ? messageDate(item.createdAt)
                : null}
              active={visible.start >= 0 && index >= visible.start - 2 && index <= visible.end + 2}
            />
          {/each}
        </virtual-list>
      {/key}
      {#if session.selected}
        {#key `${session.selected}:${session.draftGeneration}`}
          <Composer
            {draft}
            {sending}
            canSend={!!draft.trim() && !!timeline?.isCurrent}
            onChange={(value) => (draft = value)}
            onSend={() => void send()}
          />
        {/key}
        {#if sendError}<text class="error">{sendError}</text>{/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  .connected {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
    background-color: var(--color-background);
    color: var(--color-text);
  }
  .server-rail {
    width: 54px;
    flex-shrink: 0;
    padding: 9px 6px;
    border-right-width: 1px;
    border-color: var(--color-border);
  }
  .current-server {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 3px;
    border-radius: 12px;
    background-color: var(--color-selected);
    border-width: 1px;
    border-color: #4b6268;
  }
  .sidebar {
    display: flex;
    flex-direction: column;
    width: 246px;
    height: 100%;
    flex-shrink: 0;
    border-right-width: 1px;
    border-color: var(--color-border);
  }
  .brand {
    display: flex;
    align-items: center;
    height: 50px;
    padding: 0 14px;
    border-bottom-width: 1px;
    border-color: var(--color-border);
  }
  .server-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--color-text-top);
  }
  .muted {
    font-size: 11px;
    color: var(--color-muted);
  }
  .search {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 4px 12px;
    height: 38px;
  }
  .search-input {
    flex-grow: 1;
    min-width: 0;
    height: 28px;
  }
  .rooms {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-height: 0;
    overflow-y: scroll;
  }
  .empty-search {
    padding: 12px;
    color: var(--color-muted);
    font-size: 12px;
  }
  .account {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 7px;
    margin: 7px;
    padding: 7px;
    background-color: var(--color-surface);
    border-radius: var(--radius-panel);
  }
  .identity {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
  }
  .account-name {
    font-size: 12px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .conversation {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
    height: 100%;
  }
  .header {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 9px;
    height: 50px;
    padding: 0 16px;
    border-bottom-width: 1px;
    border-color: var(--color-border);
  }
  .title {
    font-size: 15px;
    font-weight: 600;
    color: var(--color-text-top);
  }
  .topic {
    font-size: 13px;
    color: var(--color-muted);
    flex-grow: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .header-space {
    flex-grow: 1;
  }
  .connection {
    font-size: 10px;
    color: var(--color-muted);
  }
  .status {
    padding: 6px 18px;
  }
  .history-status {
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 3px 18px;
    border-bottom-width: 1px;
    border-color: var(--color-border);
  }
  .timeline {
    flex-grow: 1;
    min-height: 0;
    width: 100%;
  }
  .error {
    color: var(--color-error);
    font-size: 12px;
    padding: 6px 18px;
  }
</style>
