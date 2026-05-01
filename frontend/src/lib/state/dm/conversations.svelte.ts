import { createContext, untrack } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { graphql, useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  SpaceEventBusSubscriptionDocument,
  UserAvatarUserFragmentDoc,
  type UserAvatarUserFragment
} from '$lib/gql/graphql';
import { DM_SPACE_ID } from '$lib/constants';
import {
  instanceRegistry,
  type RegisteredInstance
} from '$lib/state/instance/registry.svelte';
import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
import { instanceEventBusManager } from '$lib/state/instance/eventBus.svelte';
import type { EventHandler } from '$lib/instanceEventBus.svelte';
import { mergeInstanceConversations } from './mergeConversations';

export type DMConversation = {
  id: string;
  instanceId: string;
  instanceLabel: string;
  hasUnread: boolean;
  participants: UserAvatarUserFragment[];
  currentUserId: string | undefined;
  isSelfConversation: boolean;
};

const DMConversationsQuery = graphql(`
  query GetDmConversationsForList {
    me {
      id
    }
    space(id: "DM") {
      rooms {
        id
        hasUnread
        members {
          ...UserAvatarUser
        }
      }
    }
  }
`);

function getInstanceHostname(instance: { url: string }): string {
  try {
    return new URL(instance.url).hostname;
  } catch {
    return instance.url;
  }
}

/**
 * Centralized state for the cross-instance Direct Messages list.
 *
 * Owns the conversation list, its loading state, and ingestion of DM-related
 * subscription events across all connected instances. Components read
 * {@link conversations} reactively and call mutator methods (`markRead`,
 * `bumpToTop`) — they do not issue GraphQL queries or subscribe directly.
 *
 * Lifecycle is driven by the consumer (typically the `/chat/dm` layout):
 * - Call {@link loadAll} from a `$effect` so a refetch happens whenever the
 *   set of instances changes.
 * - Call {@link wireSubscriptions} from a `$effect` (passing
 *   `instanceRegistry.instances` directly) so subscriptions re-wire when the
 *   set of instances changes; the cleanup tears everything down on unmount.
 *
 * The active conversation id (URL-derived) is supplied as a getter to
 * {@link wireSubscriptions} — invoked lazily inside subscription handlers
 * and refetches, so the read happens *when an event arrives*, not at wire
 * time. This avoids syncing URL state into the store via `$effect` (see
 * `frontend.md`).
 */
export class DMConversationsStore {
  conversations = $state<DMConversation[]>([]);

  private loadingCount = $state(0);
  isLoading = $derived(this.loadingCount > 0);

  private pendingRefetch = new SvelteMap<string, Promise<void>>();
  private getActiveConversationId: () => string | undefined = () => undefined;

  /**
   * Wire per-instance subscriptions for the supplied instances list.
   *
   * Pass the live `instanceRegistry.instances` from a `$effect` — Svelte
   * tracks that read in the calling effect, so when the registry mutates
   * (instance added/disconnected), the effect re-runs, the returned cleanup
   * tears down old subscriptions, and `wireSubscriptions` runs again with
   * the updated list.
   */
  wireSubscriptions(
    instances: RegisteredInstance[],
    getActiveConversationId: () => string | undefined
  ): () => void {
    this.getActiveConversationId = getActiveConversationId;

    const cleanups: (() => void)[] = [];
    for (const instance of instances) {
      const onActivity = (roomId: string) => {
        const active = this.getActiveConversationId();
        this.bumpToTop(instance.id, roomId, roomId !== active);
      };

      const bus = instanceEventBusManager.getBus(instance.id);
      if (bus) {
        const handler: EventHandler = (event) => {
          if (event.event?.__typename === 'NewDirectMessageNotificationEvent') {
            onActivity(event.event.roomId);
          }
        };
        bus.handlers.add(handler);
        cleanups.push(() => bus.handlers.delete(handler));
      }

      const client = graphqlClientManager.getClient(instance.id);
      const sub = client.client
        .subscription(SpaceEventBusSubscriptionDocument, { spaceId: DM_SPACE_ID })
        .subscribe((result) => {
          if (!result.data) return;
          const event = useFragment(RoomEventViewFragmentDoc, result.data.mySpaceEvents);
          if (event?.event?.__typename === 'MessagePostedEvent') {
            onActivity(event.event.roomId);
          }
        });
      cleanups.push(() => sub.unsubscribe());
    }

    return () => cleanups.forEach((c) => c());
  }

  /** Load DM conversations from a single instance and merge into the list. */
  private async loadInstance(instanceId: string): Promise<void> {
    const instance = instanceRegistry.getInstance(instanceId);
    if (!instance) return;

    const client = graphqlClientManager.getClient(instanceId);
    const result = await client.client.query(DMConversationsQuery, {}).toPromise();
    if (!result.data?.space) return;

    const meId = result.data.me?.id;
    const label = instance.name?.trim() || getInstanceHostname(instance);
    const rooms = result.data.space.rooms ?? [];
    const activeId = this.getActiveConversationId();

    const newConversations: DMConversation[] = rooms.map((room) => {
      const participants = room.members.map((m) => useFragment(UserAvatarUserFragmentDoc, m));
      const others = participants.filter((p) => p.id !== meId);
      return {
        id: room.id,
        instanceId,
        instanceLabel: label,
        // The active conversation is being viewed — don't let a refetch
        // resurrect a stale `hasUnread: true` against a recent markRead.
        hasUnread: room.id === activeId ? false : room.hasUnread,
        participants,
        currentUserId: meId,
        isSelfConversation: others.length === 0
      };
    });

    this.conversations = mergeInstanceConversations(
      this.conversations,
      instanceId,
      newConversations
    );
  }

  /**
   * Load conversations from all connected instances in parallel.
   *
   * Reads `instanceRegistry.instances` synchronously, so calling this from a
   * `$effect` makes the effect track the registry — a registry change
   * (instance added or disconnected) re-runs the effect and triggers a fresh
   * cross-instance fetch. Cheap enough for the rarity of that event.
   */
  async loadAll(): Promise<void> {
    const instances = instanceRegistry.instances;
    this.loadingCount = instances.length;

    await Promise.allSettled(
      instances.map(async (instance) => {
        try {
          await this.loadInstance(instance.id);
        } finally {
          this.loadingCount--;
        }
      })
    );
  }

  /**
   * Clear the unread flag on a conversation. Wrapped in `untrack` so callers
   * can invoke from a `$effect` without a read-write loop on `conversations`.
   */
  markRead(instanceId: string, roomId: string): void {
    untrack(() => {
      const idx = this.conversations.findIndex(
        (c) => c.id === roomId && c.instanceId === instanceId
      );
      if (idx === -1) return;
      this.conversations[idx] = { ...this.conversations[idx], hasUnread: false };
    });
  }

  /**
   * Move a conversation to the top of the list, optionally marking it unread.
   * If the conversation isn't in the list yet (e.g. a brand-new DM), this
   * triggers a per-instance refetch and re-applies the bump once it settles.
   * Concurrent bumps for the same `${instanceId}:${roomId}` share the same
   * in-flight refetch.
   */
  bumpToTop(instanceId: string, roomId: string, markUnread: boolean): void {
    const index = this.conversations.findIndex(
      (c) => c.id === roomId && c.instanceId === instanceId
    );
    if (index === -1) {
      const key = `${instanceId}:${roomId}`;
      let inflight = this.pendingRefetch.get(key);
      if (!inflight) {
        inflight = this.loadInstance(instanceId).finally(() => {
          this.pendingRefetch.delete(key);
        });
        this.pendingRefetch.set(key, inflight);
      }
      void inflight.then(() => this.bumpToTop(instanceId, roomId, markUnread));
      return;
    }

    const conv = this.conversations[index];
    if (markUnread) {
      conv.hasUnread = true;
    }
    if (index > 0) {
      this.conversations = [
        conv,
        ...this.conversations.slice(0, index),
        ...this.conversations.slice(index + 1)
      ];
    }
  }
}

export const [getDMConversationsStore, setDMConversationsStore] =
  createContext<DMConversationsStore>();
