<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { untrack } from 'svelte';
  import { scoreItem } from './quickSwitcherSearch';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { getGradientForName } from '$lib/utils/gradients';
  import { recentQuickSwitcher } from '$lib/state/recentQuickSwitcher.svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { ROOM_MEMBERS_PAGE_SIZE } from '$lib/state/room/members.svelte';
  import * as m from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import { createRoomCommandAPI } from '@chatto/api-client/rooms';
  import { createMemberDirectoryAPI, type DirectoryMember } from '@chatto/api-client/memberDirectory';
  import { createRoomDirectoryAPI, RoomDirectoryScope, RoomKind } from '@chatto/api-client/roomDirectory';

  type ServerLogo = { name: string; logoUrl?: string | null };
  type AvatarUser = Pick<DirectoryMember, 'id' | 'login' | 'displayName' | 'deleted'> & {
    avatarUrl?: string | null;
  };

  type ResultItem = {
    kind: 'room' | 'dm' | 'destination' | 'server' | 'user';
    id: string;
    label: string;
    detail: string;
    serverId: string;
    serverName: string;
    serverLogo?: ServerLogo;
    participants?: AvatarUser[];
    currentUserId?: string;
    targetUserId?: string;
    href?: string;
    icon?: string;
    score: number;
  };

  let query = $state('');
  let selectedIndex = $state(0);
  let loading = $state(false);
  let userSearchLoading = $state(false);
  let allItems = $state.raw<ResultItem[]>([]);
  let userItems = $state.raw<ResultItem[]>([]);
  let dialogEl: HTMLDialogElement | undefined;
  let inputEl: HTMLInputElement | undefined;
  let userSearchTimer: ReturnType<typeof setTimeout> | undefined;
  let userSearchRequestId = 0;

  // --- Data loading ---

  async function loadAll() {
    loading = true;
    const instances = serverRegistry.servers;
    const multiInstance = instances.length > 1;
    const items: ResultItem[] = [];

    await Promise.allSettled(
      instances.map(async (instance) => {
        const serverConnection = serverConnectionManager.getClient(instance.id);
        const store = serverRegistry.tryGetStore(instance.id);
        const serverName = store?.serverInfo.name || instance.name || getHostname(instance.url);
        const serverLabel = multiInstance ? serverName : '';
        const currentUserId = store?.currentUser.user?.id ?? undefined;
        const directory = createRoomDirectoryAPI({
          serverId: instance.id,
          baseUrl: serverConnection.connectBaseUrl,
          bearerToken: serverConnection.bearerToken
        });
        const members = createMemberDirectoryAPI({
          baseUrl: serverConnection.connectBaseUrl,
          bearerToken: serverConnection.bearerToken
        });

        const logo: ServerLogo = {
          name: serverName,
          logoUrl: store?.serverInfo.iconUrl ?? null
        };

        items.push({
          kind: 'server',
          id: `server-${instance.id}`,
          label: logo.name,
          detail: '',
          serverId: instance.id,
          serverName: logo.name,
          serverLogo: logo,
          href: resolve('/chat/[serverId]/overview', { serverId: serverIdToSegment(instance.id) }),
          score: 0
        });

        const rooms = await directory.listRooms(RoomDirectoryScope.ALL);
        await Promise.all(
          rooms.map(async (room) => {
            if (room.kind === RoomKind.DM) {
              const participants = (
                await members.listRoomMembers(room.id, '', ROOM_MEMBERS_PAGE_SIZE, 0)
              ).members.map(avatarUser);
              items.push({
                kind: 'dm',
                id: room.id,
                label: dmLabel(participants, currentUserId),
                detail: serverLabel,
                serverId: instance.id,
                serverName,
                participants,
                currentUserId,
                score: 0
              });
              return;
            }

            if (!room.isMember) return;
            items.push({
              kind: 'room',
              id: room.id,
              label: room.name,
              detail: serverLabel || logo.name,
              serverId: instance.id,
              serverName,
              serverLogo: logo,
              score: 0
            });
          })
        );
      })
    );

    items.push({
      kind: 'destination',
      id: 'notifications',
      label: 'Notifications',
      detail: '',
      serverId: '',
      serverName: '',
      href: resolve('/chat/notifications'),
      icon: 'uil--bell',
      score: 0
    });

    allItems = items;
    selectedIndex = 0;
    loading = false;
  }

  function scheduleUserSearch(raw: string) {
    if (userSearchTimer) clearTimeout(userSearchTimer);

    const search = raw.trim();
    const requestId = ++userSearchRequestId;

    if (!quickSwitcher.visible || !search || search.startsWith('#')) {
      userItems = [];
      userSearchLoading = false;
      return;
    }

    userSearchLoading = true;
    userSearchTimer = setTimeout(() => {
      void loadUserResults(search, requestId);
    }, 200);
  }

  function handleQueryInput(e: Event) {
    query = (e.currentTarget as HTMLInputElement).value;
    selectedIndex = 0;
    scheduleUserSearch((e.currentTarget as HTMLInputElement).value);
  }

  async function loadUserResults(search: string, requestId: number) {
    const instances = serverRegistry.servers;
    const multiInstance = instances.length > 1;
    const items: ResultItem[] = [];

    await Promise.allSettled(
      instances.map(async (instance) => {
        const serverConnection = serverConnectionManager.getClient(instance.id);
        const store = serverRegistry.tryGetStore(instance.id);
        const serverName = store?.serverInfo.name || instance.name || getHostname(instance.url);
        const serverLabel = multiInstance ? serverName : '';

        if (!store?.permissions.canStartDMs) return;

        const currentUserId = store.currentUser.user?.id ?? undefined;
        const api = createMemberDirectoryAPI({
          baseUrl: serverConnection.connectBaseUrl,
          bearerToken: serverConnection.bearerToken
        });
        const result = await api.listServerMembers(search, 20, 0);
        for (const member of result.members) {
          const user = avatarUser(member);
          items.push({
            kind: 'user',
            id: user.id,
            label: user.displayName || user.login,
            detail: [user.login ? `@${user.login}` : '', serverLabel].filter(Boolean).join(' · '),
            serverId: instance.id,
            serverName,
            participants: [user],
            currentUserId,
            targetUserId: user.id,
            score: 0
          });
        }
      })
    );

    if (requestId !== userSearchRequestId) return;
    userItems = items;
    selectedIndex = 0;
    userSearchLoading = false;
  }

  function getHostname(url: string): string {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  }

  function dmLabel(participants: AvatarUser[], currentUserId: string | undefined): string {
    const others = participants.filter((p) => p.id !== currentUserId);
    if (others.length === 0) {
      const self = participants.find((p) => p.id === currentUserId);
      return self ? self.displayName || self.login : 'You';
    }
    return others.map((p) => p.displayName || p.login).join(', ');
  }

  // --- Filtering ---

  let filtered = $derived.by(() => {
    const raw = query.trim();
    const recentUrls = recentQuickSwitcher.urls;
    const recentSet = new Set(recentUrls);
    const searchableItems = [...allItems.filter((item) => item.kind !== 'dm'), ...userItems];

    if (!raw) {
      // Split into recent and non-recent groups
      const recent: ResultItem[] = [];
      const rest: ResultItem[] = [];

      for (const item of allItems) {
        const url = itemUrl(item);
        if (url && recentSet.has(url)) {
          recent.push(item);
        } else {
          rest.push(item);
        }
      }

      // Sort recents by their recency order
      recent.sort((a, b) => {
        const ai = recentUrls.indexOf(itemUrl(a)!);
        const bi = recentUrls.indexOf(itemUrl(b)!);
        return ai - bi;
      });

      // Sort rest by kind then alphabetically
      const kindOrder: Record<ResultItem['kind'], number> = {
        destination: 0,
        server: 1,
        room: 2,
        dm: 3,
        user: 4
      };
      rest.sort((a, b) => kindOrder[a.kind] - kindOrder[b.kind] || a.label.localeCompare(b.label));

      return [...recent, ...rest];
    }

    const isChannelFilter = raw.startsWith('#');
    const q = isChannelFilter ? raw.slice(1) : raw;
    const pool = isChannelFilter
      ? allItems.filter((item) => item.kind === 'room')
      : searchableItems;

    if (isChannelFilter && !q) {
      return [...pool].sort((a, b) => a.label.localeCompare(b.label));
    }

    // Multi-token fuzzy match across label, detail, and server name.
    const scored: ResultItem[] = [];
    for (const item of pool) {
      const matchScore = scoreItem(q, item);
      if (matchScore === null) continue;

      let best = matchScore;
      // Boost recent destinations
      const url = itemUrl(item);
      if (url) {
        const recentIndex = recentUrls.indexOf(url);
        if (recentIndex !== -1) {
          best += 300 - recentIndex * 20;
        }
      }
      scored.push({ ...item, score: best });
    }

    scored.sort((a, b) => b.score - a.score);

    return scored;
  });

  // --- Visibility ---

  function syncQuickSwitcherDialog(node: HTMLDialogElement) {
    dialogEl = node;
    const visible = quickSwitcher.visible;

    untrack(() => {
      if (visible) {
        query = '';
        selectedIndex = 0;
        allItems = [];
        userItems = [];
        scheduleUserSearch('');
        if (!node.open) node.showModal();
        requestAnimationFrame(() => inputEl?.focus());
        loadAll();
      } else {
        if (userSearchTimer) clearTimeout(userSearchTimer);
        userItems = [];
        userSearchLoading = false;
        userSearchRequestId++;
        if (node.open) node.close();
      }
    });
  }

  function registerInput(node: HTMLInputElement) {
    inputEl = node;
  }

  // --- Navigation ---

  function itemUrl(item: ResultItem): string | undefined {
    if ((item.kind === 'destination' || item.kind === 'server') && item.href) return item.href;
    if (item.kind === 'dm')
      return resolve('/chat/[serverId]/[roomId]', {
        serverId: serverIdToSegment(item.serverId),
        roomId: item.id
      });
    if (item.kind === 'room')
      return resolve('/chat/[serverId]/[roomId]', {
        serverId: serverIdToSegment(item.serverId),
        roomId: item.id
      });
    return undefined;
  }

  async function startDMFromUser(item: ResultItem) {
    if (!item.targetUserId) throw new Error('Missing DM target');

    const conn = serverConnectionManager.getClient(item.serverId);
    const room = await createRoomCommandAPI({
      serverId: item.serverId,
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    }).startDM(item.targetUserId === item.currentUserId ? [] : [item.targetUserId]);

    const roomId = room?.id;
    if (!roomId) throw new Error('Failed to start DM');

    return roomId;
  }

  async function select(item: ResultItem) {
    quickSwitcher.close();

    if (item.kind === 'user') {
      try {
        const roomId = await startDMFromUser(item);
        const url = resolve('/chat/[serverId]/[roomId]', {
          serverId: serverIdToSegment(item.serverId),
          roomId
        });
        recentQuickSwitcher.record(url);
        goto(
          resolve('/chat/[serverId]/[roomId]', {
            serverId: serverIdToSegment(item.serverId),
            roomId
          })
        );
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to start DM');
      }
      return;
    }

    const url = itemUrl(item);
    if (url) {
      recentQuickSwitcher.record(url);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- itemUrl() returns resolved app routes
      goto(url);
    }
  }

  // --- Keyboard ---

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, filtered.length - 1);
      scrollSelectedIntoView();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, 0);
      scrollSelectedIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const item = filtered[selectedIndex];
      if (item) select(item);
    }
  }

  function scrollSelectedIntoView() {
    requestAnimationFrame(() => {
      const el = dialogEl?.querySelector(`[data-index="${selectedIndex}"]`);
      el?.scrollIntoView({ block: 'nearest' });
    });
  }

  function close() {
    quickSwitcher.close();
  }

  // --- Kind labels ---

  const kindLabels = $derived<Record<ResultItem['kind'], string>>({
    destination: m['quick_switcher.kind.destination'](),
    server: m['quick_switcher.kind.server'](),
    room: m['quick_switcher.kind.room'](),
    dm: m['quick_switcher.kind.dm'](),
    user: m['quick_switcher.kind.user']()
  });

  function isRecent(item: ResultItem): boolean {
    const url = itemUrl(item);
    return url !== undefined && recentQuickSwitcher.urls.includes(url);
  }

  function showGroupHeader(index: number): string | null {
    if (query.trim()) return null;
    const item = filtered[index];
    if (!item) return null;
    const prev = index > 0 ? filtered[index - 1] : null;
    const itemIsRecent = isRecent(item);
    const prevIsRecent = prev ? isRecent(prev) : false;

    // Transition from recent to non-recent section
    if (!itemIsRecent && (index === 0 || prevIsRecent)) return kindLabels[item.kind];
    // First item or kind change within non-recent section
    if (itemIsRecent && (index === 0 || !prevIsRecent)) return m['quick_switcher.recent']();
    if (!itemIsRecent && prev && prev.kind !== item.kind) return kindLabels[item.kind];
    return null;
  }

  function dmAvatarParticipants(item: ResultItem): AvatarUser[] {
    if (!item.participants) return [];
    const others = item.participants.filter((p) => p.id !== item.currentUserId);
    return others.length === 0 ? item.participants.slice(0, 1) : others.slice(0, 2);
  }

  function userAvatarParticipant(item: ResultItem): AvatarUser | null {
    return item.participants?.[0] ?? null;
  }

  function avatarUser(user: AvatarUser): AvatarUser {
    return {
      id: user.id,
      login: user.login,
      displayName: user.displayName,
      deleted: user.deleted,
      avatarUrl: user.avatarUrl ?? null
    };
  }
</script>

{#snippet avatar(user: AvatarUser)}
  {#if user.avatarUrl}
    <SkeletonImg
      loading="lazy"
      src={user.avatarUrl}
      alt={user.login}
      class="h-5 w-5 rounded-full object-cover"
    />
  {:else}
    <span
      class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-200 text-[10px] font-semibold text-muted"
      aria-label={user.login}
    >
      {getAvatarInitials(user.displayName, user.login)}
    </span>
  {/if}
{/snippet}

<!-- Outer wrapper replicates ContextMenu.svelte's container exactly -->
<dialog
  {@attach syncQuickSwitcherDialog}
  onclose={() => quickSwitcher.close()}
  onkeydown={(e) => {
    if (e.key === 'Escape') e.stopPropagation();
  }}
  oncancel={(e) => {
    e.preventDefault();
    close();
  }}
  onclick={(e) => {
    if (e.target === dialogEl) close();
  }}
  class="quick-switcher m-auto mt-[15vh] max-h-none max-w-none overflow-visible border-none bg-transparent p-0 text-inherit backdrop:bg-black/50"
>
  {#if quickSwitcher.visible}
    <div
      class="flex w-140 max-w-[90vw] flex-col gap-1 rounded-lg border border-text/10 bg-surface-100 p-1 text-sm shadow-xl"
    >
      <!-- Search section -->
      <div class="menu-section">
        <div class="flex items-center gap-2 px-3 py-1.5">
          <span class="sidebar-icon iconify text-muted uil--search"></span>
          <input
            {@attach registerInput}
            bind:value={query}
            oninput={handleQueryInput}
            onkeydown={handleKeydown}
            type="text"
            placeholder={m['quick_switcher.placeholder']()}
            class="flex-1 bg-transparent text-text outline-none placeholder:text-muted"
          />
          {#if loading || userSearchLoading}
            <span class="sidebar-icon iconify animate-spin text-muted uil--spinner-alt"></span>
          {/if}
          <kbd class="rounded border border-text/10 px-1.5 py-0.5 text-xs text-muted">Esc</kbd>
        </div>
      </div>

      <!-- Results section -->
      <div class="max-h-80 overflow-y-auto menu-section">
        <nav class="sidebar-nav">
          {#if filtered.length === 0 && !loading && !userSearchLoading}
            <p class="px-3 py-6 text-center text-muted">{m['quick_switcher.no_results']()}</p>
          {:else}
            {#each filtered as item, i (`${item.serverId}:${item.kind}:${item.id}`)}
              {@const header = showGroupHeader(i)}

              {#if header}
                <div class="px-3 pt-2 pb-0.5 text-xs font-medium text-muted uppercase">
                  {header}
                </div>
              {/if}

              <button
                data-index={i}
                type="button"
                class={['sidebar-item text-left', i === selectedIndex ? 'bg-surface-100' : '']}
                onclick={() => select(item)}
                onpointerenter={() => (selectedIndex = i)}
              >
                {#if item.kind === 'destination' && item.icon}
                  <span class="sidebar-icon iconify text-muted {item.icon}"></span>
                {:else if item.kind === 'user'}
                  {@const user = userAvatarParticipant(item)}
                  <span class="sidebar-icon">
                    {#if user}
                      {@render avatar(user)}
                    {:else}
                      <span class="sidebar-icon iconify text-muted uil--user"></span>
                    {/if}
                  </span>
                {:else if item.kind === 'dm' && item.participants}
                  <span class="sidebar-icon">
                    <div class="flex -space-x-2">
                      {#each dmAvatarParticipants(item) as participant (participant.id)}
                        {@render avatar(participant)}
                      {/each}
                    </div>
                  </span>
                {:else if item.serverLogo}
                  {@const logo = item.serverLogo}
                  <span
                    class="inline-flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded text-[10px] font-bold"
                    style:background={logo.logoUrl ? undefined : getGradientForName(logo.name)}
                  >
                    {#if logo.logoUrl}
                      <SkeletonImg
                        src={logo.logoUrl}
                        alt={logo.name}
                        class="h-full w-full object-cover"
                      />
                    {:else}
                      <span class="text-white">{logo.name[0]?.toUpperCase() ?? '?'}</span>
                    {/if}
                  </span>
                {:else}
                  <span class="sidebar-icon text-muted">#</span>
                {/if}

                <span class="min-w-0 flex-1 truncate">
                  {#if item.kind === 'room'}<span class="text-muted">#</span
                    >{/if}{item.label}{#if item.detail}<span class="text-muted"
                      >&nbsp;· {item.detail}</span
                    >{/if}
                </span>

                {#if !query.trim()}
                  <span class="shrink-0 text-xs text-muted">{kindLabels[item.kind]}</span>
                {/if}
              </button>
            {/each}
          {/if}
        </nav>
      </div>
    </div>
  {/if}
</dialog>

<style>
  dialog.quick-switcher[open] {
    animation: qs-fade-in 100ms ease-out;
  }

  dialog.quick-switcher[open]::backdrop {
    animation: qs-backdrop-in 100ms ease-out;
  }

  @keyframes qs-fade-in {
    from {
      opacity: 0;
      transform: translateY(-10px) scale(0.98);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }

  @keyframes qs-backdrop-in {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
</style>
