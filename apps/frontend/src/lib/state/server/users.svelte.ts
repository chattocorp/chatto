import { SvelteDate, SvelteMap, SvelteSet } from 'svelte/reactivity';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { StaleResponseError } from '$lib/api-client/connect';
import type { UserSummary } from '$lib/api-client/userSummary';
import type { DirectoryMember as MemberView } from '$lib/api-client/memberDirectory';
import { scheduleCustomStatusExpiry } from '$lib/utils/customStatusExpiry';

export type UserReader = (ids: string[], minimumCursor?: string) => Promise<DirectoryMember[]>;

/** The single public-profile owner for one server connection. Room membership,
 * pagination, and presence subscriptions have separate owners. Values are replaced,
 * never mutated in place. Realtime writes supersede pending snapshot reads. */
export class UserStore {
  readonly #members = new SvelteMap<string, DirectoryMember>();
  readonly #deleted = new SvelteSet<string>();
  readonly #pending = new SvelteMap<string, { completion: Promise<void>; cursor?: string }>();
  #generation = 0;
  #disposed = false;
  #revision = 0;
  readonly #revisions = new SvelteMap<string, number>();
  readonly #statusExpiry = new SvelteMap<string, () => void>();

  get size(): number { return this.#members.size; }
  get(id: string): DirectoryMember | undefined { return this.#members.get(id); }
  has(id: string): boolean { return this.#members.has(id); }
  isDeleted(id: string): boolean { return this.#deleted.has(id); }
  keys() { return this.#members.keys(); }
  values() { return this.#members.values(); }
  entries() { return this.#members.entries(); }
  [Symbol.iterator]() { return this.entries(); }

  /** Accept an authoritative profile, fencing older reads for the same identity. */
  set(id: string, member: DirectoryMember): this {
    if (this.#disposed || !id || member.user?.id !== id) return this;
    if (member.user.deleted) { this.delete(id); return this; }
    this.#pending.delete(id);
    this.#deleted.delete(id);
    this.#revisions.set(id, ++this.#revision);
    this.#members.set(id, new DirectoryMember(member));
    this.#statusExpiry.get(id)?.();
    const expiresAt = member.user.customStatus?.expiresAt?.toDate().toISOString();
    if (expiresAt) {
      this.#statusExpiry.set(id, scheduleCustomStatusExpiry({ emoji: '', text: '', expiresAt }, () => {
        const current = this.get(id)?.clone();
        if (current?.user?.customStatus?.expiresAt?.toDate().toISOString() !== expiresAt) return;
        current.user.customStatus = undefined;
        this.set(id, current);
      }));
    } else this.#statusExpiry.delete(id);
    return this;
  }

  /** Seed incidental response data only when no profile, read, or tombstone exists. */
  seed(member: DirectoryMember): void {
    const id = member.user?.id;
    if (id && !this.has(id) && !this.#deleted.has(id) && !this.#pending.has(id)) this.set(id, member);
  }

  /** Account removal is a tombstone until the next authoritative reset or write. */
  delete(id: string): boolean {
    this.#statusExpiry.get(id)?.();
    this.#statusExpiry.delete(id);
    this.#revisions.set(id, ++this.#revision);
    this.#pending.delete(id);
    this.#deleted.add(id);
    return this.#members.delete(id);
  }

  /** Profile-change invalidation permits a fresh read; account deletion does not. */
  invalidate(id: string): void {
    this.#statusExpiry.get(id)?.();
    this.#statusExpiry.delete(id);
    this.#revisions.set(id, ++this.#revision);
    this.#pending.delete(id);
    this.#members.delete(id);
  }

  /** Reset the authorized snapshot and reject every older read, including empty batches. */
  clear(): void {
    this.#generation++;
    for (const cancel of this.#statusExpiry.values()) cancel();
    this.#statusExpiry.clear();
    this.#pending.clear();
    this.#members.clear();
    this.#deleted.clear();
    this.#revisions.clear();
  }

  /** A disposed connection can never publish again, even through a retained API facade. */
  dispose(): void { this.clear(); this.#disposed = true; }

  /** Ingest a list/detail response without overwriting profiles changed since it began.
   * The returned rows use current identities; deleted accounts stay absent. */
  async readSnapshot(
    read: () => Promise<DirectoryMember[]>,
    seedOnly = false
  ): Promise<DirectoryMember[]> {
    const generation = this.#generation;
    const revision = this.#revision;
    if (this.#disposed) throw new StaleResponseError(false);
    const members = await read();
    if (generation !== this.#generation) throw new StaleResponseError(false);
    for (const member of members) {
      const id = member.user?.id;
      if (id && !this.#deleted.has(id) && (this.#revisions.get(id) ?? 0) <= revision) {
        if (seedOnly) this.seed(member);
        else this.set(id, member);
      }
    }
    return members.flatMap((member) => {
      const current = member.user?.id ? this.get(member.user.id) : undefined;
      return current ? [current] : [];
    });
  }

  missing(ids: Iterable<string>): string[] {
    return [...new SvelteSet(ids)].filter((id) => id && !this.has(id) && !this.#deleted.has(id));
  }

  /** Share missing-profile reads across all consumers, in batches of at most 100.
   * Opaque cursors are not ordered: only an omitted result from a different boundary
   * needs a retry. A cached profile remains valid until explicit invalidation. */
  async resolve(ids: string[], read: UserReader, cursor?: string): Promise<DirectoryMember[]> {
    if (this.#disposed) throw new StaleResponseError(false);
    const generation = this.#generation;
    const weaker = ids.filter((id) => {
      const pending = this.#pending.get(id);
      return pending && pending.cursor !== cursor;
    });
    const missing = this.missing(ids).filter((id) => !this.#pending.has(id));
    for (let offset = 0; offset < missing.length; offset += 100) {
      const batch = missing.slice(offset, offset + 100);
      const request = Promise.resolve().then(async () => {
        if (generation !== this.#generation) throw new StaleResponseError(false);
        const current = batch.filter((id) => this.#pending.get(id)?.completion === request);
        const users = current.length ? await read(current, cursor) : [];
        if (generation !== this.#generation) throw new StaleResponseError(false);
        const byId = new SvelteMap(users.flatMap((member) => member.user?.id ? [[member.user.id, member] as const] : []));
        for (const id of batch) {
          if (this.#pending.get(id)?.completion !== request) {
            if (!this.has(id) && !this.#deleted.has(id)) throw new StaleResponseError(false);
            continue;
          }
          const member = byId.get(id);
          if (member) this.set(id, member);
        }
      }).finally(() => {
        for (const id of batch) if (this.#pending.get(id)?.completion === request) this.#pending.delete(id);
      });
      for (const id of batch) this.#pending.set(id, { completion: request, cursor });
    }
    await Promise.all([...new SvelteSet(ids.flatMap((id) => this.#pending.get(id)?.completion ?? []))]);
    if (generation !== this.#generation) throw new StaleResponseError(false);
    if (this.missing(weaker).length) return this.resolve(ids, read, cursor);
    return [...new SvelteSet(ids)].flatMap((id) => this.get(id) ?? []);
  }
}

// Registry keys carry the connection scope; replacing credentials cannot reuse a profile owner.
const stores: Record<string, Record<string, UserStore>> = Object.create(null);
/** Resolve the owner for an explicit server and connection scope. The standalone
 * scope supports isolated API clients that do not have a ServerConnection. */
export function getUserStore(serverId: string, scope = 'standalone'): UserStore {
  const sessions = stores[serverId] ??= Object.create(null);
  return sessions[scope] ??= new UserStore();
}

/** Clear a server's private data without replacing owners retained by current consumers. */
export function clearUserStores(serverId: string): void {
  for (const store of Object.values(stores[serverId] ?? {})) store.clear();
}

/** Dispose exactly one connection; outstanding readers keep a permanently fenced owner. */
export function disposeUserStore(serverId: string, scope: string): void {
  stores[serverId]?.[scope]?.dispose();
  if (stores[serverId]) delete stores[serverId][scope];
  if (Object.keys(stores[serverId] ?? {}).length === 0) delete stores[serverId];
}

export function resetUserStoresForTests(): void {
  for (const [serverId, sessions] of Object.entries(stores)) {
    for (const store of Object.values(sessions)) store.dispose();
    delete stores[serverId];
  }
}

/** Adapt legacy render summaries at an ingestion boundary, preserving optional metadata. */
export function memberFromSummary(summary: UserSummary | MemberView, previous?: DirectoryMember): DirectoryMember {
  const member = previous?.clone() ?? new DirectoryMember();
  const user = member.user?.clone();
  member.user = new DirectoryMember({ user: {
    ...user,
    id: summary.id, login: summary.login, displayName: summary.displayName,
    deleted: summary.deleted, avatarUrl: summary.avatarUrl ?? '',
    bot: summary.bot ?? (summary.isBot ? { ownerUserId: '' } : undefined),
    ...('bio' in summary ? { bio: summary.bio ?? '' } : {}),
    ...('timezone' in summary ? { timezone: summary.timezone ?? '' } : {})
  } }).user;
  if ('roles' in summary) {
    member.roles = [...summary.roles];
    member.createdAt = summary.createdAt ? Timestamp.fromDate(new SvelteDate(summary.createdAt)) : undefined;
    if (member.user) {
      member.user.presenceStatus = summary.presenceStatus;
      member.user.customStatus = summary.customStatus ? new DirectoryMember({ user: { customStatus: {
        ...summary.customStatus,
        expiresAt: summary.customStatus.expiresAt ? Timestamp.fromDate(new SvelteDate(summary.customStatus.expiresAt)) : undefined
      } } }).user?.customStatus : undefined;
    }
  }
  return member;
}
