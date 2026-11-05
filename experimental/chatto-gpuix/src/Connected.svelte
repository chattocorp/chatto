<script lang="ts">
  import { untrack } from 'svelte';
  import { window_size } from 'gpuix-svelte';
  import type { Server } from '@chatto/client';
  import { RoomKind } from '@chatto/client';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import Avatar from './ui/Avatar.svelte';
  import PaneHeader from './ui/PaneHeader.svelte';
  import IconButton from './ui/IconButton.svelte';
  import Icon from './ui/Icon.svelte';
  import ChannelRow from './ui/ChannelRow.svelte';
  import SidebarSection from './ui/SidebarSection.svelte';
  import Composer from './ui/Composer.svelte';
  import { scaled } from './ui/scale.svelte.ts';
  import { inputTheme } from './ui/theme.ts';
  import MessageTimeline from './MessageTimeline.svelte';
  import ThreadPanel from './ThreadPanel.svelte';
  import type { NativeSession } from './session.svelte.ts';

  let threadId = $state('');
  const compactThread = $derived(!!threadId && window_size.width < scaled(1100));
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
  const timeline = $derived.by(() => {
    const roomId = session.selected;
    const owner = server;
    // Store access can evict another SDK timeline. Keep its lifecycle writes
    // outside Svelte's derived evaluation while tracking the selected identity.
    return roomId ? untrack(() => owner.rooms.messages(roomId)) : null;
  });
  const viewer = $derived(server.viewerUser);
  let draft = $state('');
  let sending = $state(false);
  let sendError = $state('');
  let search = $state('');
  const matchingRooms = $derived(
    rooms.filter((room) => roomLabel(room).toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  );
  const matchingOtherRooms = $derived(
    otherRooms.filter((room) => matchingRooms.some((match) => match.id === room.id))
  );
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
    threadId = '';
    session.select(id);
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

<div class="connected">
  <div class="sidebar">
    <PaneHeader title={server.projection.server?.name || 'Chatto'} testid="server-pane-header" />
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
        ><text class="muted">{viewer?.login ? `@${viewer.login}` : ''}</text>
      </div>
      <IconButton label="Sign out" icon="logout" onclick={() => void session.signOut()} />
    </div>
  </div>
  {#if !compactThread}<div class="conversation">
      <PaneHeader
        title={selectedRoom ? roomLabel(selectedRoom) : 'Select a conversation'}
        description={selectedRoom?.description}
        icon={selectedRoom?.type === RoomKind.DM ? 'message' : 'hash'}
        testid="channel-pane-header"
      />
      {#if session.error}<text class="error">{session.error}</text>{/if}
      {#key `${server.serverId}:${session.selected}:${session.draftGeneration}`}
        <MessageTimeline
          {server}
          {timeline}
          scope={session.selected}
          onOpenThread={(id) => (threadId = id)}
        />
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
    </div>{/if}
  {#if threadId && session.selected}
    {#key `${server.serverId}:${session.selected}:${threadId}:${session.draftGeneration}`}
      <ThreadPanel
        {server}
        roomId={session.selected}
        rootId={threadId}
        compact={compactThread}
        onClose={() => (threadId = '')}
      />
    {/key}
  {/if}
</div>

<style>
  .connected {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
    background-color: var(--color-background);
    color: var(--color-text);
  }
  .sidebar {
    display: flex;
    flex-direction: column;
    width: var(--size-246);
    height: 100%;
    flex-shrink: 0;
    border-right-width: var(--size-1);
    border-color: var(--color-border);
  }
  .muted {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .search {
    display: flex;
    align-items: center;
    gap: var(--size-5);
    padding: var(--size-4) var(--size-12);
    height: var(--size-38);
  }
  .search-input {
    font-family: var(--font-sans);
    font-size: var(--text-body);
    flex-grow: 1;
    min-width: 0;
    height: var(--size-28);
  }
  .rooms {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-height: 0;
    overflow-y: scroll;
  }
  .empty-search {
    padding: var(--size-12);
    color: var(--color-muted);
    font-size: var(--text-small);
  }
  .account {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-7);
    margin: var(--size-7);
    padding: var(--size-7);
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
    font-size: var(--text-small);
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
  .error {
    color: var(--color-error);
    font-size: var(--text-small);
    padding: var(--size-6) var(--size-18);
  }
</style>
