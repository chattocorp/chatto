/** Per-server emoji history, with separate lists for the full picker and quick reactions. */
import { PINNED_REACTIONS } from '$lib/emoji';
import { Codecs, serverSlot, type StorageSlot } from '@chatto/client/storage/slot';

/** Maximum number of choices shown in the full picker's Recently Used section. */
export const MAX_RECENT_EMOJIS = 16;
const MAX_RECENT_REACTIONS = 2;

// Validate entries on read so one corrupt entry does not discard the list.
const emojiListCodec = Codecs.json<unknown[]>((value): value is unknown[] => Array.isArray(value));

function isCustomReaction(emoji: unknown): emoji is string {
  return (
    typeof emoji === 'string' &&
    emoji.length > 0 &&
    !PINNED_REACTIONS.some((pinned) => pinned === emoji)
  );
}

/** Owns independent general-picker and reaction-picker histories for one server. */
export class RecentEmojisStore {
  /** Full-picker choices, newest first. Includes profile-status selections. */
  recent = $state<string[]>([]);
  private reactions = $state<string[]>([]);
  private storage: StorageSlot<unknown[]>;
  private reactionStorage: StorageSlot<unknown[]>;

  constructor(serverId: string) {
    this.storage = serverSlot(serverId, 'recentEmojis', [], emojiListCodec);
    this.recent = this.storage
      .get()
      .filter((e): e is string => typeof e === 'string')
      .slice(0, MAX_RECENT_EMOJIS);

    // Do not import general history: it includes profile-status choices.
    this.reactionStorage = serverSlot(serverId, 'recentReactions', [], emojiListCodec);
    const stored = this.reactionStorage.get().filter(isCustomReaction);
    this.reactions = stored
      .filter((emoji, index) => stored.indexOf(emoji) === index)
      .slice(0, MAX_RECENT_REACTIONS);
  }

  /** Record a selection from any full emoji picker. */
  record(emoji: string): void {
    this.recent = [emoji, ...this.recent.filter((previous) => previous !== emoji)].slice(
      0,
      MAX_RECENT_EMOJIS
    );
    this.storage.set(this.recent);
  }

  /** Record a message reaction-picker choice, independent of the request result. */
  recordReaction(emoji: string): void {
    if (!isCustomReaction(emoji)) return;
    this.reactions = [emoji, ...this.reactions.filter((previous) => previous !== emoji)].slice(
      0,
      MAX_RECENT_REACTIONS
    );
    this.reactionStorage.set(this.reactions);
  }

  /**
   * Pinned reactions followed by up to two recent reaction-picker choices.
   * Read state in the caller's reactive context. A shared $derived field can
   * become inert when the component that first creates this store is destroyed.
   */
  get quickReactions(): readonly string[] {
    return [...PINNED_REACTIONS, ...this.reactions];
  }
}

// Keep this identity cache non-reactive: first access inside a derivation must
// not invalidate the caller. The store owns the reactive state.
let stores: Record<string, RecentEmojisStore | undefined> = Object.create(null);

/**
 * Get (or lazily create) the recent-emojis store for a server.
 * Derive the lookup separately from state reads: Svelte does not track state
 * created within the same derivation that reads it.
 */
export function getRecentEmojis(serverId: string): RecentEmojisStore {
  return (stores[serverId] ??= new RecentEmojisStore(serverId));
}

/** Test-only: clear the store cache so a fresh instance is built per test. */
export function __resetRecentEmojisForTests(): void {
  stores = Object.create(null);
}
