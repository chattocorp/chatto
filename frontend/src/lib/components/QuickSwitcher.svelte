<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { scoreItem } from './quickSwitcherSearch';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { graphql, useFragment } from '$lib/gql';
  import {
    RoomType,
    UserAvatarUserFragmentDoc,
    type UserAvatarUserFragment
  } from '$lib/gql/graphql';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { getGradientForName } from '$lib/utils/gradients';
  import { recentQuickSwitcher } from '$lib/state/recentQuickSwitcher.svelte';
  import { startDMWith } from '$lib/dm/startDM';

  type SpaceLogo = { name: string; logoUrl?: string | null };

  type ResultItem = {
    kind: 'space' | 'room' | 'dm' | 'user' | 'destination';
    id: string;
    label: string;
    detail: string;
    instanceId: string;
    instanceName: string;
    spaceId?: string;
    spaceLogo?: SpaceLogo;
    participants?: UserAvatarUserFragment[];
    currentUserId?: string;
    href?: string;
    icon?: string;
    score: number;
  };

  let visible = $state(false);
  let query = $state('');
  let selectedIndex = $state(0);
  let loading = $state(false);
  let allItems = $state.raw<ResultItem[]>([]);
  let userSearchItems = $state.raw<ResultItem[]>([]);
  let dialogEl: HTMLDialogElement | undefined = $state();
  let inputEl: HTMLInputElement | undefined = $state();

  // Per-instance context needed for user search (populated during loadAll)
  type InstanceContext = {
    instanceId: string;
    instanceLabel: string;
    instanceName: string;
    spaceIds: string[];
    currentUserId: string | undefined;
    dmUserIds: Set<string>;
  };
  let instanceContexts: InstanceContext[] = [];

  // --- GraphQL queries ---

  const SpacesQuery = graphql(`
    query QuickSwitcherSpaces {
      me {
        spaces {
          id
          name
          logoUrl(width: 96, height: 96)
        }
      }
      viewer {
        canListSpaces
      }
    }
  `);

  const RoomsQuery = graphql(`
    query QuickSwitcherRooms($spaceId: ID!) {
      me {
        rooms(spaceId: $spaceId) {
          id
          name
          type
        }
      }
    }
  `);

  const DMsQuery = graphql(`
    query QuickSwitcherDMs {
      me {
        id
      }
      space(id: "DM") {
        rooms {
          id
          members {
            ...UserAvatarUser
          }
        }
      }
    }
  `);

  const SpaceMembersSearchQuery = graphql(`
    query QuickSwitcherSpaceMembersSearch($spaceId: ID!, $search: String!, $limit: Int!) {
      space(id: $spaceId) {
        members(search: $search, limit: $limit) {
          users {
            ...UserAvatarUser
          }
        }
      }
    }
  `);

  // --- Data loading ---

  async function loadAll() {
    loading = true;
    const instances = instanceRegistry.instances;
    const multiInstance = instances.length > 1;
    const items: ResultItem[] = [];
    const contexts: InstanceContext[] = [];
    const opts = { requestPolicy: 'network-only' as const };
    let anyCanListSpaces = false;

    await Promise.allSettled(
      instances.map(async (instance) => {
        const client = graphqlClientManager.getClient(instance.id).client;
        const store = instanceRegistry.tryGetStore(instance.id);
        const instanceName = store?.instance.name || instance.name || getHostname(instance.url);
        const instanceLabel = multiInstance ? instanceName : '';

        // Fetch spaces and DMs in parallel (allSettled so one failing doesn't block the other)
        const [spacesSettled, dmsSettled] = await Promise.allSettled([
          client.query(SpacesQuery, {}, opts).toPromise(),
          client.query(DMsQuery, {}, opts).toPromise()
        ]);

        const spacesResult = spacesSettled.status === 'fulfilled' ? spacesSettled.value : null;
        const dmsResult = dmsSettled.status === 'fulfilled' ? dmsSettled.value : null;

        if (spacesResult?.data?.viewer?.canListSpaces) anyCanListSpaces = true;

        // Spaces
        type SpaceInfo = { id: string; name: string; logoUrl?: string | null };
        const spaces: SpaceInfo[] = [];

        if (spacesResult?.data?.me) {
          for (const space of spacesResult.data.me.spaces) {
            const logo: SpaceLogo = { name: space.name, logoUrl: space.logoUrl };
            spaces.push({ id: space.id, name: space.name, logoUrl: space.logoUrl });
            items.push({
              kind: 'space',
              id: space.id,
              label: space.name,
              detail: instanceLabel,
              instanceId: instance.id,
              instanceName,
              spaceId: space.id,
              spaceLogo: logo,
              score: 0
            });
          }
        }

        // DMs — track which user IDs already have a DM conversation
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local dedup set, not read reactively
        const dmUserIds = new Set<string>();
        const currentUserId = dmsResult?.data?.me?.id ?? undefined;

        if (dmsResult?.data?.space) {
          for (const room of dmsResult.data.space.rooms ?? []) {
            const participants = room.members.map((m) =>
              useFragment(UserAvatarUserFragmentDoc, m)
            );
            const others = participants.filter((p) => p.id !== currentUserId);
            const isSelf = others.length === 0;

            // Track user IDs that already have DM conversations
            for (const p of others) dmUserIds.add(p.id);

            let label: string;
            if (isSelf) {
              const self = participants.find((p) => p.id === currentUserId);
              label = self ? self.displayName || self.login : 'You';
            } else {
              label = others
                .map((p) => p.displayName || p.login)
                .join(', ');
            }

            items.push({
              kind: 'dm',
              id: room.id,
              label,
              detail: instanceLabel,
              instanceId: instance.id,
              instanceName,
              participants,
              currentUserId,
              score: 0
            });
          }
        }

        // Save context for later user searches
        contexts.push({
          instanceId: instance.id,
          instanceLabel,
          instanceName,
          spaceIds: spaces.map((s) => s.id),
          currentUserId,
          dmUserIds
        });

        // Fetch rooms for all spaces in parallel
        await Promise.allSettled(
          spaces.map(async (space) => {
            const roomsResult = await client
              .query(RoomsQuery, { spaceId: space.id }, opts)
              .toPromise();

            if (roomsResult.data?.me) {
              const logo: SpaceLogo = { name: space.name, logoUrl: space.logoUrl };
              for (const room of roomsResult.data.me.rooms) {
                // Skip DMs surfaced through the merged primary-space response
                // (#330 phase 3) — they're already added as kind='dm' items
                // above via DMsQuery, with the right label and avatar. Without
                // this filter they'd double up as kind='room' items with an
                // empty name, rendering as "# · <primary-space-name>" and
                // impersonating the actual primary-space channels.
                if (room.type === RoomType.Dm) continue;
                items.push({
                  kind: 'room',
                  id: room.id,
                  label: room.name,
                  detail: space.name,
                  instanceId: instance.id,
                  instanceName,
                  spaceId: space.id,
                  spaceLogo: logo,
                  score: 0
                });
              }
            }
          })
        );
      })
    );

    // Well-known destinations (one entry each, not per-instance)
    if (anyCanListSpaces) {
      items.push({
        kind: 'destination',
        id: 'browse-spaces',
        label: 'Browse Spaces',
        detail: '',
        instanceId: '',
        instanceName: '',
        href: resolve('/chat/spaces'),
        icon: 'uil--compass',
        score: 0
      });
    }
    items.push({
      kind: 'destination',
      id: 'notifications',
      label: 'Notifications',
      detail: '',
      instanceId: '',
      instanceName: '',
      href: '/chat/notifications',
      icon: 'uil--bell',
      score: 0
    });
    items.push({
      kind: 'destination',
      id: 'connected-instances',
      label: 'Connected Instances',
      detail: '',
      instanceId: '',
      instanceName: '',
      href: resolve('/instances'),
      icon: 'uil--globe',
      score: 0
    });

    instanceContexts = contexts;
    allItems = items;
    loading = false;
  }

  // --- Server-side user search (debounced) ---

  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  let searchGeneration = 0;

  function scheduleUserSearch(q: string) {
    clearTimeout(searchTimer);
    if (!q) {
      userSearchItems = [];
      return;
    }
    searchTimer = setTimeout(() => searchUsers(q), 200);
  }

  async function searchUsers(q: string) {
    const gen = ++searchGeneration;
    const opts = { requestPolicy: 'network-only' as const };
    const results: ResultItem[] = [];
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local dedup set, not read reactively
    const seenUserIds = new Set<string>();

    await Promise.allSettled(
      instanceContexts.map(async (ctx) => {
        const client = graphqlClientManager.getClient(ctx.instanceId).client;

        // Search all spaces this user belongs to in parallel
        await Promise.allSettled(
          ctx.spaceIds.map(async (spaceId) => {
            const result = await client
              .query(SpaceMembersSearchQuery, { spaceId, search: q, limit: 10 }, opts)
              .toPromise();

            const users = result?.data?.space?.members?.users;
            if (!users) return;

            for (const rawUser of users) {
              const user = useFragment(UserAvatarUserFragmentDoc, rawUser);
              if (user.id === ctx.currentUserId) continue;
              if (ctx.dmUserIds.has(user.id)) continue;
              const key = `${ctx.instanceId}:${user.id}`;
              if (seenUserIds.has(key)) continue;
              seenUserIds.add(key);

              results.push({
                kind: 'user',
                id: user.id,
                label: user.displayName || user.login,
                detail: ctx.instanceLabel,
                instanceId: ctx.instanceId,
                instanceName: ctx.instanceName,
                participants: [user],
                currentUserId: ctx.currentUserId,
                score: 0
              });
            }
          })
        );
      })
    );

    // Only apply if this is still the latest search
    if (gen === searchGeneration) {
      userSearchItems = results;
    }
  }

  // Trigger user search when query changes
  $effect(() => {
    const q = query.trim();
    scheduleUserSearch(q);
  });

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
      const kindOrder: Record<ResultItem['kind'], number> = { destination: 0, space: 1, room: 2, dm: 3, user: 4 };
      rest.sort(
        (a, b) => kindOrder[a.kind] - kindOrder[b.kind] || a.label.localeCompare(b.label)
      );

      return [...recent, ...rest];
    }

    const isChannelFilter = raw.startsWith('#');
    const q = isChannelFilter ? raw.slice(1) : raw;
    const pool = isChannelFilter ? allItems.filter((item) => item.kind === 'room') : allItems;

    if (isChannelFilter && !q) {
      return [...pool].sort((a, b) => a.label.localeCompare(b.label));
    }

    // Multi-token fuzzy match across label, space name (detail), and instance name.
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

    // Append server-side user search results (already filtered, no duplicates with DMs)
    if (!isChannelFilter && userSearchItems.length > 0) {
      for (const user of userSearchItems) {
        scored.push(user);
      }
    }

    return scored;
  });

  $effect(() => {
    void filtered;
    selectedIndex = 0;
  });

  // --- Visibility ---

  export function open() {
    visible = true;
  }

  $effect(() => {
    if (visible) {
      query = '';
      selectedIndex = 0;
      allItems = [];
      userSearchItems = [];
      dialogEl?.showModal();
      requestAnimationFrame(() => inputEl?.focus());
      loadAll();
    } else {
      dialogEl?.close();
    }
  });

  // --- Navigation ---

  function itemUrl(item: ResultItem): string | undefined {
    if (item.kind === 'destination' && item.href) return item.href;
    if (item.kind === 'dm') return resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceIdToSegment(item.instanceId), roomId: item.id });
    if (item.kind === 'room' && item.spaceId) return resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceIdToSegment(item.instanceId), roomId: item.id });
    if (item.kind === 'space') return resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(item.instanceId) });
    return undefined;
  }

  function select(item: ResultItem) {
    visible = false;

    if (item.kind === 'user') {
      startDMWith(item.instanceId, item.id);
      return;
    }

    const url = itemUrl(item);
    if (url) {
      recentQuickSwitcher.record(url);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- url from itemUrl() is already resolved
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
    visible = false;
  }

  // --- Kind labels ---

  const kindLabels: Record<ResultItem['kind'], string> = {
    destination: 'Go to',
    space: 'Space',
    room: 'Room',
    dm: 'DM',
    user: 'User'
  };

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
    if (itemIsRecent && (index === 0 || !prevIsRecent)) return 'Recent';
    if (!itemIsRecent && prev && prev.kind !== item.kind) return kindLabels[item.kind];
    return null;
  }

  function dmAvatarParticipants(item: ResultItem): UserAvatarUserFragment[] {
    if (!item.participants) return [];
    const others = item.participants.filter((p) => p.id !== item.currentUserId);
    return (others.length === 0 ? item.participants.slice(0, 1) : others.slice(0, 2));
  }
</script>

<!-- Outer wrapper replicates ContextMenu.svelte's container exactly -->
<dialog
  bind:this={dialogEl}
  onclose={() => (visible = false)}
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
  {#if visible}
  <div class="flex w-140 max-w-[90vw] flex-col gap-1 rounded-lg border border-text/10 bg-surface-100 p-1 text-sm shadow-xl">
    <!-- Search section -->
    <div class="menu-section">
      <div class="flex items-center gap-2 px-3 py-1.5">
        <span class="sidebar-icon iconify text-muted uil--search"></span>
        <input
          bind:this={inputEl}
          bind:value={query}
          onkeydown={handleKeydown}
          type="text"
          placeholder="Go to space, room, conversation, or user..."
          class="flex-1 bg-transparent text-text outline-none placeholder:text-muted"
        />
        {#if loading}
          <span class="sidebar-icon iconify animate-spin text-muted uil--spinner-alt"></span>
        {/if}
        <kbd class="rounded border border-text/10 px-1.5 py-0.5 text-xs text-muted">Esc</kbd>
      </div>
    </div>

    <!-- Results section -->
    <div class="menu-section max-h-80 overflow-y-auto">
      <nav class="sidebar-nav">
        {#if filtered.length === 0 && !loading}
          <p class="px-3 py-6 text-center text-muted">No results</p>
        {:else}
          {#each filtered as item, i (`${item.instanceId}:${item.kind}:${item.id}`)}
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
                i === selectedIndex ? 'bg-surface-100' : ''
              ]}
              onclick={() => select(item)}
              onpointerenter={() => (selectedIndex = i)}
            >
              {#if item.kind === 'destination' && item.icon}
                <span class="sidebar-icon iconify text-muted {item.icon}"></span>
              {:else if (item.kind === 'dm' || item.kind === 'user') && item.participants}
                <span class="sidebar-icon">
                  <div class="flex -space-x-2">
                    {#each dmAvatarParticipants(item) as participant (participant.id)}
                      <UserAvatar user={participant} size="xs" showPresence={false} />
                    {/each}
                  </div>
                </span>
              {:else if item.spaceLogo}
                {@const logo = item.spaceLogo}
                <span class="inline-flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded text-[10px] font-bold" style:background={logo.logoUrl ? undefined : getGradientForName(logo.name)}>
                  {#if logo.logoUrl}
                    <SkeletonImg src={logo.logoUrl} alt={logo.name} class="h-full w-full object-cover" />
                  {:else}
                    <span class="text-white">{logo.name[0]?.toUpperCase() ?? '?'}</span>
                  {/if}
                </span>
              {:else}
                <span class="sidebar-icon text-muted">#</span>
              {/if}

              <span class="min-w-0 flex-1 truncate">
                {#if item.kind === 'room'}<span class="text-muted">#</span>{/if}{item.label}{#if item.detail}<span class="text-muted">&nbsp;· {item.detail}</span>{/if}
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
