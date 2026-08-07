import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { untrack } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import { createMemberDirectoryAPI, type DirectoryMember } from '$lib/api-client/memberDirectory';
import {
  createMessageSearchAPI,
  MessageSearchOrder,
  type MessageSearchResult
} from '$lib/api-client/messageSearch';
import { createRoomCommandAPI } from '$lib/api-client/rooms';
import { useDebounce } from '$lib/hooks/useDebounce.svelte';
import { m } from '$lib/i18n/messages';
import { buildMessageLinkPath } from '$lib/messageLinks';
import { serverIdToSegment } from '$lib/navigation';
import { buildDirectMessagePresentation } from '$lib/render/users';
import { quickSwitcher } from '$lib/state/globals.svelte';
import { recentQuickSwitcher } from '$lib/state/recentQuickSwitcher.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { isNavigationVisibleRoom } from '$lib/state/server/rooms.svelte';
import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
import { toast } from '$lib/ui/toast';
import { scoreItem } from './quickSwitcherSearch';

export type QuickSwitcherAvatarUser = Pick<
  DirectoryMember,
  'id' | 'login' | 'displayName' | 'deleted'
> & {
  avatarUrl?: string | null;
};

type ServerLogo = { name: string; logoUrl?: string | null };

export type QuickSwitcherItem = {
  kind: 'room' | 'dm' | 'destination' | 'server' | 'user' | 'message';
  id: string;
  label: string;
  detail: string;
  serverId: string;
  serverName: string;
  serverLogo?: ServerLogo;
  participants?: QuickSwitcherAvatarUser[];
  currentUserId?: string;
  targetUserId?: string;
  href?: string;
  icon?: string;
  message?: MessageSearchResult;
  score: number;
};

const SEARCH_DEBOUNCE_MS = 200;
const MESSAGE_SEARCH_SERVER_TIMEOUT_MS = 3_000;

class SearchChannel<T> {
  items = $state.raw<T[]>([]);
  loading = $state(false);

  #requestId = 0;
  #debounce = useDebounce();

  schedule(query: string | null, load: (query: string, requestId: number) => void): void {
    this.#debounce.cancel();
    const requestId = ++this.#requestId;
    if (!query) {
      this.items = [];
      this.loading = false;
      return;
    }

    this.loading = true;
    this.#debounce.run(() => load(query, requestId), SEARCH_DEBOUNCE_MS);
  }

  replace(requestId: number, items: T[]): boolean {
    if (!this.isCurrent(requestId)) return false;
    this.items = items;
    return true;
  }

  finish(requestId: number): void {
    if (this.isCurrent(requestId)) this.loading = false;
  }

  isCurrent(requestId: number): boolean {
    return requestId === this.#requestId;
  }

  fence(): void {
    this.#debounce.cancel();
    this.#requestId++;
    this.items = [];
    this.loading = false;
  }
}

/**
 * Owns the Quick Switcher's per-mount catalog, search, privacy, selection, and
 * navigation lifecycle. DOM focus and dialog behavior remain in the component.
 */
export class QuickSwitcherModel {
  query = $state('');
  selectedIndex = $state(0);

  #allItems = $state.raw<QuickSwitcherItem[]>([]);
  #userSearch = new SearchChannel<QuickSwitcherItem>();
  #messageSearch = new SearchChannel<QuickSwitcherItem>();
  #messageSearchServerKey = '';

  kindLabels = $derived<Record<QuickSwitcherItem['kind'], string>>({
    destination: m('quick_switcher.kind.destination'),
    server: m('quick_switcher.kind.server'),
    room: m('quick_switcher.kind.room'),
    dm: m('quick_switcher.kind.dm'),
    user: m('quick_switcher.kind.user'),
    message: m('quick_switcher.kind.message')
  });

  filtered = $derived.by(() => {
    const raw = this.query.trim();
    const recentUrls = recentQuickSwitcher.urls;
    const recentSet = new SvelteSet(recentUrls);
    const searchableItems = [
      ...this.#allItems.filter((item) => item.kind !== 'dm'),
      ...this.#userSearch.items
    ];

    if (raw.startsWith('?')) return this.#messageSearch.items;

    if (!raw) {
      const recent: QuickSwitcherItem[] = [];
      const rest: QuickSwitcherItem[] = [];
      for (const item of this.#allItems) {
        const url = this.#itemUrl(item);
        (url && recentSet.has(url) ? recent : rest).push(item);
      }
      recent.sort(
        (a, b) => recentUrls.indexOf(this.#itemUrl(a)!) - recentUrls.indexOf(this.#itemUrl(b)!)
      );
      const kindOrder: Record<QuickSwitcherItem['kind'], number> = {
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
    const query = isChannelFilter ? raw.slice(1) : raw;
    const pool = isChannelFilter
      ? this.#allItems.filter((item) => item.kind === 'room')
      : searchableItems;
    if (isChannelFilter && !query) {
      return [...pool].sort((a, b) => a.label.localeCompare(b.label));
    }

    const scored: QuickSwitcherItem[] = [];
    for (const item of pool) {
      const matchScore = scoreItem(query, item);
      if (matchScore === null) continue;
      const recentIndex = recentUrls.indexOf(this.#itemUrl(item) ?? '');
      scored.push({
        ...item,
        score: matchScore + (recentIndex === -1 ? 0 : 300 - recentIndex * 20)
      });
    }
    return scored.sort((a, b) => b.score - a.score);
  });

  get loading(): boolean {
    return this.#userSearch.loading || this.#messageSearch.loading;
  }

  activate(): void {
    this.query = '';
    this.selectedIndex = 0;
    this.#allItems = [];
    this.#userSearch.fence();
    this.#messageSearch.fence();
  }

  deactivate(): void {
    this.#userSearch.fence();
    this.#messageSearch.fence();
  }

  setQuery(raw: string): void {
    this.query = raw;
    this.selectedIndex = 0;

    const userQuery = raw.trim();
    this.#userSearch.schedule(
      quickSwitcher.visible && userQuery && !userQuery.startsWith('#') && !userQuery.startsWith('?')
        ? userQuery
        : null,
      (query, requestId) => void this.#loadUserResults(query, requestId)
    );

    this.#scheduleMessageSearch(raw);
  }

  moveSelection(delta: number): void {
    this.selectedIndex = Math.max(
      0,
      Math.min(this.selectedIndex + delta, this.filtered.length - 1)
    );
  }

  selectIndex(index: number): void {
    this.selectedIndex = index;
  }

  selectCurrent(): void {
    const item = this.filtered[this.selectedIndex];
    if (item) void this.select(item);
  }

  async select(item: QuickSwitcherItem): Promise<void> {
    quickSwitcher.close();

    if (item.kind === 'user') {
      try {
        const roomId = await this.#startDMFromUser(item);
        const url = resolve('/chat/[serverId]/[roomId]', {
          serverId: serverIdToSegment(item.serverId),
          roomId
        });
        recentQuickSwitcher.record(url);
        await goto(url);
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Failed to start DM');
      }
      return;
    }

    const url = this.#itemUrl(item);
    if (!url) return;
    if (item.kind !== 'message') recentQuickSwitcher.record(url);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- itemUrl returns paths from resolve() or buildMessageLinkPath()
    await goto(url);
  }

  groupHeader(index: number): string | null {
    if (this.query.trim()) return null;
    const item = this.filtered[index];
    if (!item) return null;
    const previous = index > 0 ? this.filtered[index - 1] : null;
    const recent = this.#isRecent(item);
    const previousRecent = previous ? this.#isRecent(previous) : false;

    if (!recent && (index === 0 || previousRecent)) return this.kindLabels[item.kind];
    if (recent && (index === 0 || !previousRecent)) return m('quick_switcher.recent');
    if (!recent && previous && previous.kind !== item.kind) return this.kindLabels[item.kind];
    return null;
  }

  /** Track the canonical cross-server navigation inputs and rebuild the local catalog. */
  syncCatalog(): void {
    if (!quickSwitcher.visible) return;
    void serverRegistry.servers;
    for (const instance of serverRegistry.servers) {
      const store = serverRegistry.tryGetStore(instance.id);
      void store?.navigation.rooms;
      void store?.navigation.isInitialLoading;
    }
    untrack(() => this.#loadCatalog());
  }

  /** Subscribe to each server's message-search privacy boundary for this open palette. */
  syncPrivacy(): (() => void) | undefined {
    if (!quickSwitcher.visible) return;
    const instances = serverRegistry.servers;
    const serverKey = instances.map((instance) => instance.id).join('\0');
    const stores = instances.flatMap((instance) => {
      const store = serverRegistry.tryGetStore(instance.id);
      return store ? [{ serverId: instance.id, store }] : [];
    });

    return untrack(() => {
      if (this.#messageSearchServerKey && this.#messageSearchServerKey !== serverKey) {
        this.#restartMessageSearch(true);
      }
      this.#messageSearchServerKey = serverKey;
      const unsubscribes = stores.map(({ serverId, store }) =>
        store.messageSearch.subscribePrivacyInvalidation((matches, force) => {
          if (!quickSwitcher.visible) return;
          const affected = this.#messageSearch.items.some(
            (item) => item.serverId === serverId && item.message && matches(item.message)
          );
          this.#restartMessageSearch(force || affected);
        })
      );
      return () => unsubscribes.forEach((unsubscribe) => unsubscribe());
    });
  }

  #restartMessageSearch(clearResults: boolean): void {
    if (clearResults) this.#messageSearch.fence();
    this.#scheduleMessageSearch(this.query);
  }

  #scheduleMessageSearch(raw: string): void {
    const trimmed = raw.trim();
    const query = trimmed.startsWith('?') ? trimmed.slice(1).trim() : '';
    this.#messageSearch.schedule(
      quickSwitcher.visible && query ? query : null,
      (search, requestId) => void this.#loadMessageResults(search, requestId)
    );
  }

  #loadCatalog(): void {
    const instances = serverRegistry.servers;
    const multiInstance = instances.length > 1;
    const items: QuickSwitcherItem[] = [];

    for (const instance of instances) {
      const store = serverRegistry.tryGetStore(instance.id);
      const serverName = store?.serverInfo.name || instance.name || getHostname(instance.url);
      const serverLabel = multiInstance ? serverName : '';
      const currentUserId = store?.currentUser.user?.id ?? undefined;
      const logo: ServerLogo = { name: serverName, logoUrl: store?.serverInfo.iconUrl ?? null };

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
            m('common.you')
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
      label: m('ui.notifications'),
      detail: '',
      serverId: '',
      serverName: '',
      href: resolve('/chat/notifications'),
      icon: 'icon-[uil--bell]',
      score: 0
    });
    this.#allItems = items;
    this.selectedIndex = 0;
  }

  async #loadUserResults(search: string, requestId: number): Promise<void> {
    const instances = serverRegistry.servers;
    const multiInstance = instances.length > 1;
    const items: QuickSwitcherItem[] = [];

    await Promise.allSettled(
      instances.map(async (instance) => {
        const store = serverRegistry.tryGetStore(instance.id);
        if (!store?.permissions.canStartDMs) return;

        const serverName = store.serverInfo.name || instance.name || getHostname(instance.url);
        const result = await serverConnectionManager
          .getClient(instance.id)
          .getAPI(createMemberDirectoryAPI)
          .listUsers(search, 20, 0);
        for (const member of result.members) {
          const user = avatarUser(member);
          items.push({
            kind: 'user',
            id: user.id,
            label: user.displayName || user.login,
            detail: [user.login ? `@${user.login}` : '', multiInstance ? serverName : '']
              .filter(Boolean)
              .join(' · '),
            serverId: instance.id,
            serverName,
            participants: [user],
            currentUserId: store.currentUser.user?.id ?? undefined,
            targetUserId: user.id,
            score: 0
          });
        }
      })
    );

    if (this.#userSearch.replace(requestId, items)) this.selectedIndex = 0;
    this.#userSearch.finish(requestId);
  }

  async #loadMessageResults(search: string, requestId: number): Promise<void> {
    const instances = [...serverRegistry.servers];
    const resultsByServer: Record<string, QuickSwitcherItem[]> = {};
    const publish = (serverId: string, items: QuickSwitcherItem[]) => {
      if (!this.#messageSearch.isCurrent(requestId)) return;
      if (!serverRegistry.servers.some((instance) => instance.id === serverId)) return;
      const selected = this.#messageSearch.items[this.selectedIndex];
      resultsByServer[serverId] = items;
      const accumulated = Object.values(resultsByServer)
        .flat()
        .sort(
          (a, b) =>
            b.score - a.score ||
            (b.message?.createdAt ?? '').localeCompare(a.message?.createdAt ?? '') ||
            a.id.localeCompare(b.id)
        );
      if (!this.#messageSearch.replace(requestId, accumulated)) return;
      const preservedIndex = selected
        ? accumulated.findIndex(
            (item) => item.serverId === selected.serverId && item.id === selected.id
          )
        : -1;
      this.selectedIndex = preservedIndex >= 0 ? preservedIndex : 0;
    };

    const searches = instances.map(async (instance): Promise<QuickSwitcherItem[]> => {
      const store = serverRegistry.tryGetStore(instance.id);
      if (!store?.serverInfo.supportsFeature('messageSearch')) return [];
      await store.messageSearch.ensureStatus();
      if (!store.messageSearch.available) return [];

      const serverName = store.serverInfo.name || instance.name || getHostname(instance.url);
      try {
        const page = await serverConnectionManager
          .getClient(instance.id)
          .getAPI(createMessageSearchAPI)
          .searchMessages({
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
    const boundedSearches = searches.map((promise) =>
      resolveWithin(promise, MESSAGE_SEARCH_SERVER_TIMEOUT_MS, [])
    );
    boundedSearches.forEach((promise, index) => {
      const serverId = instances[index]!.id;
      void promise.then(
        (items) => publish(serverId, items),
        () => publish(serverId, [])
      );
    });
    await Promise.all(boundedSearches);
    this.#messageSearch.finish(requestId);
  }

  #itemUrl(item: QuickSwitcherItem): string | undefined {
    if ((item.kind === 'destination' || item.kind === 'server') && item.href) return item.href;
    if (item.kind === 'dm' || item.kind === 'room') {
      return resolve('/chat/[serverId]/[roomId]', {
        serverId: serverIdToSegment(item.serverId),
        roomId: item.id
      });
    }
    if (item.kind === 'message' && item.message) {
      return buildMessageLinkPath(
        item.serverId,
        item.message.roomId,
        item.message.id,
        item.message.threadRootEventId
      );
    }
  }

  #isRecent(item: QuickSwitcherItem): boolean {
    const url = this.#itemUrl(item);
    return url !== undefined && recentQuickSwitcher.urls.includes(url);
  }

  async #startDMFromUser(item: QuickSwitcherItem): Promise<string> {
    if (!item.targetUserId) throw new Error('Missing DM target');
    const room = await serverConnectionManager
      .getClient(item.serverId)
      .getAPI(createRoomCommandAPI)
      .startDM(item.targetUserId === item.currentUserId ? [] : [item.targetUserId]);
    if (!room?.id) throw new Error('Failed to start DM');
    return room.id;
  }
}

function avatarUser(user: QuickSwitcherAvatarUser): QuickSwitcherAvatarUser {
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    deleted: user.deleted,
    avatarUrl: user.avatarUrl ?? null
  };
}

function getHostname(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
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
