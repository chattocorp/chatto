<script lang="ts">
  import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
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
  import { buildDirectMessagePresentation } from '$lib/render/users';
  import { recentQuickSwitcher } from '$lib/state/recentQuickSwitcher.svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';

  import { isNavigationVisibleRoom } from '$lib/state/server/rooms.svelte';
  import * as m from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { createMemberDirectoryAPI, type DirectoryMember } from '$lib/api-client/memberDirectory';
  import {
    createMessageSearchAPI,
    MessageSearchOrder,
    type MessageSearchResult
  } from '$lib/api-client/messageSearch';

  type ServerLogo = { name: string; logoUrl?: string | null };
  type AvatarUser = Pick<DirectoryMember, 'id' | 'login' | 'displayName' | 'deleted'> & {
    avatarUrl?: string | null;
  };

  type ResultItem = {
    kind: 'room' | 'dm' | 'destination' | 'server' | 'user' | 'message';
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
    message?: MessageSearchResult;
    score: number;
  };

  const MESSAGE_SEARCH_SERVER_TIMEOUT_MS = 3_000;

  let query = $state('');
  let selectedIndex = $state(0);
  let userSearchLoading = $state(false);
  let messageSearchLoading = $state(false);
  let allItems = $state.raw<ResultItem[]>([]);
  let userItems = $state.raw<ResultItem[]>([]);
  let messageItems = $state.raw<ResultItem[]>([]);
  let dialogEl: HTMLDialogElement | undefined;
  let inputEl: HTMLInputElement | undefined;
  const userSearchDebounce = useDebounce();
  const messageSearchDebounce = useDebounce();
  let userSearchRequestId = 0;
  let messageSearchRequestId = 0;
  let messageSearchServerKey = '';

  // --- Data loading ---

  function loadAll() {
    const instances = serverRegistry.servers;
    const multiInstance = instances.length > 1;
    const items: ResultItem[] = [];

    for (const instance of instances) {
      const store = serverRegistry.tryGetStore(instance.id);
      const serverName = store?.serverInfo.name || instance.name || getHostname(instance.url);
      const serverLabel = multiInstance ? serverName : '';
      const currentUserId = store?.currentUser.user?.id ?? undefined;
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

      for (const room of store?.navigation.rooms ?? []) {
        if (room.type === RoomKind.DM) {
          if (!isNavigationVisibleRoom(room)) continue;
          const participants = room.members.map(avatarUser);
          const presentation = buildDirectMessagePresentation(
            participants,
            currentUserId,
            m['common.you']()
          );
          items.push({
            kind: 'dm',
            id: room.id,
            label: presentation.label,
            detail: serverLabel,
            serverId: instance.id,
            serverName,
            participants: presentation.visibleParticipants.slice(0, 2),
            score: 0
          });
          continue;
        }

        if (!room.viewerIsMember) continue;
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
      }
    }

    items.push({
      kind: 'destination',
      id: 'notifications',
      label: m['ui.notifications'](),
      detail: '',
      serverId: '',
      serverName: '',
      href: resolve('/chat/notifications'),
      icon: 'uil--bell',
      score: 0
    });

    allItems = items;
    selectedIndex = 0;
  }

  function scheduleUserSearch(raw: string) {
    userSearchDebounce.cancel();

    const search = raw.trim();
    const requestId = ++userSearchRequestId;

    if (!quickSwitcher.visible || !search || search.startsWith('#') || search.startsWith('?')) {
      userItems = [];
      userSearchLoading = false;
      return;
    }

    userSearchLoading = true;
    userSearchDebounce.run(() => {
      void loadUserResults(search, requestId);
    }, 200);
  }

  function scheduleMessageSearch(raw: string) {
    messageSearchDebounce.cancel();

    const trimmed = raw.trim();
    const search = trimmed.startsWith('?') ? trimmed.slice(1).trim() : '';
    const requestId = ++messageSearchRequestId;

    if (!quickSwitcher.visible || !search) {
      messageItems = [];
      messageSearchLoading = false;
      return;
    }

    messageSearchLoading = true;
    messageSearchDebounce.run(() => {
      void loadMessageResults(search, requestId);
    }, 200);
  }

  function handleQueryInput(e: Event) {
    const value = (e.currentTarget as HTMLInputElement).value;
    query = value;
    selectedIndex = 0;
    scheduleUserSearch(value);
    scheduleMessageSearch(value);
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
        const api = serverConnection.getAPI(createMemberDirectoryAPI);
        const result = await api.listUsers(search, 20, 0);
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

  async function loadMessageResults(search: string, requestId: number) {
    const instances = [...serverRegistry.servers];
    const resultsByServer: Record<string, ResultItem[]> = {};

    function publish(serverId: string, items: ResultItem[]) {
      if (requestId !== messageSearchRequestId) return;
      if (!serverRegistry.servers.some((instance) => instance.id === serverId)) return;
      const selected = messageItems[selectedIndex];
      resultsByServer[serverId] = items;
      const accumulated = Object.values(resultsByServer)
        .flat()
        .sort(
          (a, b) =>
            b.score - a.score ||
            (b.message?.createdAt ?? '').localeCompare(a.message?.createdAt ?? '') ||
            a.id.localeCompare(b.id)
        );
      messageItems = accumulated;
      const preservedIndex = selected
        ? accumulated.findIndex(
            (item) => item.serverId === selected.serverId && item.id === selected.id
          )
        : -1;
      selectedIndex = preservedIndex >= 0 ? preservedIndex : 0;
    }

    const searches = instances.map(async (instance): Promise<ResultItem[]> => {
      const store = serverRegistry.tryGetStore(instance.id);
      if (!store?.serverInfo.supportsFeature('messageSearch')) return [];

      await store.messageSearch.ensureStatus();
      if (!store.messageSearch.available) return [];

      const serverName = store.serverInfo.name || instance.name || getHostname(instance.url);
      const api = serverConnectionManager.getClient(instance.id).getAPI(createMessageSearchAPI);
      try {
        const page = await api.searchMessages({
          query: search,
          order: MessageSearchOrder.RELEVANCE,
          pageSize: 10
        });
        return page.results.map((message) => ({
          kind: 'message',
          id: message.id,
          label: message.body,
          detail: [
            message.actor?.displayName || message.actor?.login,
            message.roomName ? `#${message.roomName}` : null,
            serverName
          ]
            .filter(Boolean)
            .join(' · '),
          serverId: instance.id,
          serverName,
          message,
          score: message.relevanceScore
        }));
      } catch {
        return [];
      }
    });
    const boundedSearches = searches.map((searchPromise) =>
      resolveWithin(searchPromise, MESSAGE_SEARCH_SERVER_TIMEOUT_MS, [])
    );

    boundedSearches.forEach((searchPromise, index) => {
      const serverId = instances[index]!.id;
      void searchPromise.then(
        (items) => publish(serverId, items),
        () => publish(serverId, [])
      );
    });

    await Promise.all(boundedSearches);

    if (requestId !== messageSearchRequestId) return;
    messageSearchLoading = false;
  }

  function resolveWithin<T>(promise: Promise<T>, timeoutMs: number, fallback: T): Promise<T> {
    return new Promise((resolve) => {
      let settled = false;
      const timeout = setTimeout(() => {
        settled = true;
        resolve(fallback);
      }, timeoutMs);
      void promise.then(
        (value) => {
          if (settled) return;
          settled = true;
          clearTimeout(timeout);
          resolve(value);
        },
        () => {
          if (settled) return;
          settled = true;
          clearTimeout(timeout);
          resolve(fallback);
        }
      );
    });
  }

  function getHostname(url: string): string {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  }

  // --- Filtering ---

  let filtered = $derived.by(() => {
    const raw = query.trim();
    const recentUrls = recentQuickSwitcher.urls;
    const recentSet = new Set(recentUrls);
    const searchableItems = [...allItems.filter((item) => item.kind !== 'dm'), ...userItems];

    if (raw.startsWith('?')) return messageItems;

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
        user: 4,
        message: 5
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
        messageItems = [];
        scheduleUserSearch('');
        scheduleMessageSearch('');
        if (!node.open) node.showModal();
      } else {
        userSearchDebounce.cancel();
        messageSearchDebounce.cancel();
        userItems = [];
        messageItems = [];
        userSearchLoading = false;
        messageSearchLoading = false;
        userSearchRequestId++;
        messageSearchRequestId++;
        if (node.open) node.close();
      }
    });
  }

  // Rebuild from the canonical per-server room stores when the switcher opens or a store refreshes.
  $effect(() => {
    if (!quickSwitcher.visible) return;
    void serverRegistry.servers;
    for (const instance of serverRegistry.servers) {
      const store = serverRegistry.tryGetStore(instance.id);
      void store?.navigation.rooms;
      void store?.navigation.isInitialLoading;
    }
    untrack(loadAll);
  });

  // Purge and fence the palette's transient plaintext through the same
  // realtime privacy invalidations as each server's dedicated Search state.
  $effect(() => {
    if (!quickSwitcher.visible) return;
    const instances = serverRegistry.servers;
    const serverKey = instances.map((instance) => instance.id).join('\0');
    const stores = instances.flatMap((instance) => {
      const store = serverRegistry.tryGetStore(instance.id);
      return store ? [{ serverId: instance.id, store }] : [];
    });
    const unsubscribes = untrack(() => {
      if (messageSearchServerKey && messageSearchServerKey !== serverKey) {
        messageItems = [];
        scheduleMessageSearch(query);
      }
      messageSearchServerKey = serverKey;
      return stores.map(({ serverId, store }) =>
        store.messageSearch.subscribePrivacyInvalidation((matches, force) => {
          if (!quickSwitcher.visible) return;
          const affected = messageItems.some(
            (item) => item.serverId === serverId && item.message && matches(item.message)
          );
          if (force || affected) messageItems = [];
          scheduleMessageSearch(query);
        })
      );
    });
    return () => unsubscribes.forEach((unsubscribe) => unsubscribe());
  });

  function registerInput(node: HTMLInputElement) {
    inputEl = node;
    queueMicrotask(() => node.focus());
    return () => {
      if (inputEl === node) inputEl = undefined;
    };
  }

  // --- Navigation ---

  function itemUrl(item: ResultItem): string | undefined {
    if ((item.kind === 'destination' || item.kind === 'server') && item.href) return item.href;
    if (item.kind === 'dm' || item.kind === 'room') {
      return resolve('/chat/[serverId]/[roomId]', {
        serverId: serverIdToSegment(item.serverId),
        roomId: item.id
      });
    }
    if (item.kind === 'message' && item.message) {
      if (item.message.threadRootEventId) {
        return resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
          serverId: serverIdToSegment(item.serverId),
          roomId: item.message.roomId,
          threadId: item.message.threadRootEventId,
          messageId: item.message.id
        });
      }
      return resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        serverId: serverIdToSegment(item.serverId),
        roomId: item.message.roomId,
        messageId: item.message.id
      });
    }
    return undefined;
  }

  async function startDMFromUser(item: ResultItem) {
    if (!item.targetUserId) throw new Error('Missing DM target');

    const conn = serverConnectionManager.getClient(item.serverId);
    const room = await conn
      .getAPI(createRoomCommandAPI)
      .startDM(item.targetUserId === item.currentUserId ? [] : [item.targetUserId]);

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
        goto(url);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to start DM');
      }
      return;
    }

    const url = itemUrl(item);
    if (!url) return;
    if (item.kind !== 'message') recentQuickSwitcher.record(url);

    if (item.kind === 'destination') {
      goto(resolve('/chat/notifications'));
    } else if (item.kind === 'server') {
      goto(resolve('/chat/[serverId]/overview', { serverId: serverIdToSegment(item.serverId) }));
    } else if (item.kind === 'dm' || item.kind === 'room') {
      goto(
        resolve('/chat/[serverId]/[roomId]', {
          serverId: serverIdToSegment(item.serverId),
          roomId: item.id
        })
      );
    } else if (item.kind === 'message' && item.message) {
      if (item.message.threadRootEventId) {
        goto(
          resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
            serverId: serverIdToSegment(item.serverId),
            roomId: item.message.roomId,
            threadId: item.message.threadRootEventId,
            messageId: item.message.id
          })
        );
      } else {
        goto(
          resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
            serverId: serverIdToSegment(item.serverId),
            roomId: item.message.roomId,
            messageId: item.message.id
          })
        );
      }
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

  // --- Kind labels ---

  const kindLabels = $derived<Record<ResultItem['kind'], string>>({
    destination: m['quick_switcher.kind.destination'](),
    server: m['quick_switcher.kind.server'](),
    room: m['quick_switcher.kind.room'](),
    dm: m['quick_switcher.kind.dm'](),
    user: m['quick_switcher.kind.user'](),
    message: m['quick_switcher.kind.message']()
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
      class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-emphasized text-[10px] font-semibold text-muted"
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
    quickSwitcher.close();
  }}
  onclick={(e) => {
    if (e.target === dialogEl) quickSwitcher.close();
  }}
  class="quick-switcher m-auto mt-[15vh] max-h-none max-w-none overflow-visible border-none bg-transparent p-0 text-inherit backdrop:bg-black/50"
>
  {#if quickSwitcher.visible}
    <div
      class="flex w-140 max-w-[90vw] flex-col gap-1 rounded-lg border border-text/10 bg-surface p-1 text-sm shadow-xl"
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
          {#if userSearchLoading || messageSearchLoading}
            <span class="sidebar-icon iconify animate-spin text-muted uil--spinner-alt"></span>
          {/if}
          <kbd class="rounded border border-text/10 px-1.5 py-0.5 text-xs text-muted">Esc</kbd>
        </div>
      </div>

      <!-- Results section -->
      <div class="max-h-80 overflow-y-auto menu-section">
        <nav class="sidebar-nav">
          {#if filtered.length === 0 && !userSearchLoading && !messageSearchLoading}
            <p class="px-3 py-6 text-center text-muted">
              {query.trim() === '?'
                ? m['quick_switcher.message_search.prompt']()
                : query.trim().startsWith('?')
                  ? m['quick_switcher.message_search.no_results']()
                  : m['quick_switcher.no_results']()}
            </p>
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
                class={[
                  'sidebar-item text-left',
                  item.kind === 'message' ? 'items-start px-2 py-2' : '',
                  i === selectedIndex ? 'bg-surface' : ''
                ]}
                onclick={() => select(item)}
                onpointerenter={() => (selectedIndex = i)}
              >
                {#if item.kind === 'message'}
                  <span
                    class="mt-0.5 sidebar-icon iconify shrink-0 text-muted uil--comment-alt-message"
                  ></span>
                {:else if item.kind === 'destination' && item.icon}
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
                      {#each item.participants as participant (participant.id)}
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

                {#if item.kind === 'message'}
                  <span class="min-w-0 flex-1">
                    <span class="line-clamp-2 leading-snug break-words whitespace-pre-line"
                      >{item.label}</span
                    >
                    {#if item.detail}
                      <span
                        data-testid="message-search-provenance"
                        class="mt-0.5 block truncate text-muted">{item.detail}</span
                      >
                    {/if}
                  </span>
                {:else}
                  <span class="min-w-0 flex-1 truncate">
                    {#if item.kind === 'room'}<span class="text-muted">#</span
                      >{/if}{item.label}{#if item.detail}<span class="text-muted"
                        >&nbsp;· {item.detail}</span
                      >{/if}
                  </span>
                {/if}

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
