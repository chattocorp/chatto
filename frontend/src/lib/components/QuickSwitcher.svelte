<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { scoreItem } from './quickSwitcherSearch';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
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
  import { quickSwitcher } from '$lib/state/globals.svelte';

  type SpaceLogo = { name: string; logoUrl?: string | null };

  type ResultItem = {
    kind: 'room' | 'dm' | 'destination';
    id: string;
    label: string;
    detail: string;
    serverId: string;
    serverName: string;
    spaceLogo?: SpaceLogo;
    participants?: UserAvatarUserFragment[];
    currentUserId?: string;
    href?: string;
    icon?: string;
    score: number;
  };

  let query = $state('');
  let selectedIndex = $state(0);
  let loading = $state(false);
  let allItems = $state.raw<ResultItem[]>([]);
  let dialogEl: HTMLDialogElement | undefined = $state();
  let inputEl: HTMLInputElement | undefined = $state();

  // --- GraphQL queries ---

  const InstanceQuery = graphql(`
    query QuickSwitcherInstance {
      server {
        config {
          serverName
          logoUrl(width: 96, height: 96)
        }
      }
    }
  `);

  const RoomsQuery = graphql(`
    query QuickSwitcherRooms {
      viewer {
        user {
          id
          rooms {
            id
            name
            type
            members {
              ...UserAvatarUser
            }
          }
        }
      }
    }
  `);

  // --- Data loading ---

  async function loadAll() {
    loading = true;
    const instances = serverRegistry.instances;
    const multiInstance = instances.length > 1;
    const items: ResultItem[] = [];
    const opts = { requestPolicy: 'network-only' as const };

    await Promise.allSettled(
      instances.map(async (instance) => {
        const client = graphqlClientManager.getClient(instance.id).client;
        const store = serverRegistry.tryGetStore(instance.id);
        const serverName = store?.instance.name || instance.name || getHostname(instance.url);
        const instanceLabel = multiInstance ? serverName : '';

        // Fetch instance metadata + this user's rooms in parallel.
        const [instanceSettled, roomsSettled] = await Promise.allSettled([
          client.query(InstanceQuery, {}, opts).toPromise(),
          client.query(RoomsQuery, {}, opts).toPromise()
        ]);

        const instanceResult = instanceSettled.status === 'fulfilled' ? instanceSettled.value : null;
        const roomsResult = roomsSettled.status === 'fulfilled' ? roomsSettled.value : null;

        const logo: SpaceLogo = {
          name: instanceResult?.data?.server?.config.serverName ?? serverName,
          logoUrl: instanceResult?.data?.server?.config.logoUrl ?? null
        };
        const currentUserId = roomsResult?.data?.viewer?.user.id ?? undefined;

        if (roomsResult?.data?.viewer?.user) {
          for (const room of roomsResult.data.viewer.user.rooms) {
            if (room.type === RoomType.Dm) {
              const participants = room.members.map((m) =>
                useFragment(UserAvatarUserFragmentDoc, m)
              );
              const others = participants.filter((p) => p.id !== currentUserId);
              const isSelf = others.length === 0;

              let label: string;
              if (isSelf) {
                const self = participants.find((p) => p.id === currentUserId);
                label = self ? self.displayName || self.login : 'You';
              } else {
                label = others.map((p) => p.displayName || p.login).join(', ');
              }

              items.push({
                kind: 'dm',
                id: room.id,
                label,
                detail: instanceLabel,
                serverId: instance.id,
                serverName,
                participants,
                currentUserId,
                score: 0
              });
              continue;
            }

            items.push({
              kind: 'room',
              id: room.id,
              label: room.name,
              detail: instanceLabel || logo.name,
              serverId: instance.id,
              serverName,
              spaceLogo: logo,
              score: 0
            });
          }
        }
      })
    );

    items.push({
      kind: 'destination',
      id: 'notifications',
      label: 'Notifications',
      detail: '',
      serverId: '',
      serverName: '',
      href: '/chat/notifications',
      icon: 'uil--bell',
      score: 0
    });

    allItems = items;
    loading = false;
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
      const kindOrder: Record<ResultItem['kind'], number> = { destination: 0, room: 2, dm: 3 };
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

    return scored;
  });

  $effect(() => {
    void filtered;
    selectedIndex = 0;
  });

  // --- Visibility ---

  $effect(() => {
    if (quickSwitcher.visible) {
      query = '';
      selectedIndex = 0;
      allItems = [];
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
    if (item.kind === 'dm') return resolve('/chat/[serverId]/(chrome)/[roomId]', { serverId: serverIdToSegment(item.serverId), roomId: item.id });
    if (item.kind === 'room') return resolve('/chat/[serverId]/(chrome)/[roomId]', { serverId: serverIdToSegment(item.serverId), roomId: item.id });
    return undefined;
  }

  function select(item: ResultItem) {
    quickSwitcher.close();

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
    quickSwitcher.close();
  }

  // --- Kind labels ---

  const kindLabels: Record<ResultItem['kind'], string> = {
    destination: 'Go to',
    room: 'Room',
    dm: 'DM'
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
          placeholder="Go to space, room, or conversation..."
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
                i === selectedIndex ? 'bg-surface-100' : ''
              ]}
              onclick={() => select(item)}
              onpointerenter={() => (selectedIndex = i)}
            >
              {#if item.kind === 'destination' && item.icon}
                <span class="sidebar-icon iconify text-muted {item.icon}"></span>
              {:else if item.kind === 'dm' && item.participants}
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
