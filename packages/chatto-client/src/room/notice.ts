/** Current notice state. The room registry owns its lifetime and privacy resets. */
import type { RoomNotice } from '@chatto/api-types/api/v1/rooms_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { batch, signal } from '../reactivity/index.js';
import { createRoomNoticesAPI, type RoomNoticesAPI } from '../api/roomNotices.js';
import type { ServerConnection } from '../server/serverConnection.js';
import { StoreEvent } from '../server/storeEvents.js';

/** Retained, memory-only notice state, with stale-response and access-loss fences. */
export class RoomNoticeStore {
  readonly #notice = signal<RoomNotice | null>(null);
  readonly #changes = new StoreEvent<[RoomNotice | null]>();
  readonly #error = signal<unknown>(null);
  readonly #supported = signal(true);
  readonly #accessEpoch = signal(0);
  readonly #api: RoomNoticesAPI;
  #generation = 0;
  #retained = 0;
  #blocked = false;
  #request?: AbortController;
  #minimumCursor?: string;
  #pendingWrites = 0;

  constructor(
    connection: ServerConnection,
    readonly roomId: string
  ) {
    this.#api = connection.getAPI(createRoomNoticesAPI);
  }

  get notice() {
    return this.#notice.get();
  }
  get error() {
    return this.#error.get();
  }
  /** False on servers whose API does not implement room notices. */
  get supported() {
    return this.#supported.get();
  }
  /** Changes when private drafts and other host copies must be discarded. */
  get accessEpoch() {
    return this.#accessEpoch.get();
  }

  /** Loads on first retain. An optional listener receives current content and accepted changes until release. */
  retain(onChange?: (notice: RoomNotice | null) => void): () => void {
    onChange?.(this.notice);
    const unsubscribe = onChange ? this.#changes.subscribe(onChange) : undefined;
    this.#retained++;
    if (this.#retained === 1) void this.refresh();
    let retained = true;
    return () => {
      if (retained) {
        retained = false;
        unsubscribe?.();
        this.#retained = Math.max(0, this.#retained - 1);
      }
    };
  }

  /** Reconciles live invalidations through a read that includes the event. */
  invalidate(minimumCursor?: string): void {
    this.#minimumCursor = minimumCursor ?? this.#minimumCursor;
    this.#generation++;
    this.#request?.abort();
    if (this.#retained) void this.refresh();
  }

  /** Explicit retry and reload. Failed reads retain content unless access was denied. */
  async refresh(): Promise<void> {
    if (this.#blocked) return;
    const generation = ++this.#generation;
    this.#request?.abort();
    const request = new AbortController();
    this.#request = request;
    try {
      const notice = await this.#api.get(this.roomId, {
        signal: request.signal,
        minimumCursor: this.#minimumCursor
      });
      if (generation !== this.#generation || this.#blocked) return;
      // A delayed write response can replace this read. Retain its fence until
      // all pending writes have returned, so a subsequent read includes it.
      if (!this.#pendingWrites) this.#minimumCursor = undefined;
      batch(() => {
        this.#setNotice(notice);
        this.#error.set(null);
        this.#supported.set(true);
      });
    } catch (error) {
      if (generation !== this.#generation || request.signal.aborted) return;
      if (error instanceof ConnectError && error.code === Code.Unimplemented) {
        this.#supported.set(false);
        this.#setNotice(null);
        return;
      }
      if (
        error instanceof ConnectError &&
        [Code.PermissionDenied, Code.NotFound, Code.Unauthenticated].includes(error.code)
      )
        this.reset(true);
      this.#error.set(error);
    }
  }

  /** Writes never retry conflicts. The editor keeps its draft and supplied version. */
  async publish(body: string, expectedVersion: string, republish: boolean): Promise<void> {
    if (this.#blocked) throw new ConnectError('Room access was revoked', Code.PermissionDenied);
    const epoch = this.accessEpoch;
    this.#pendingWrites++;
    try {
      const notice = await this.#api.publish(this.roomId, body, expectedVersion, republish);
      this.#acceptWrite(notice, epoch);
    } finally {
      this.#pendingWrites--;
    }
  }

  /** Clears content with the editor's version and applies the canonical response. */
  async clear(expectedVersion: string): Promise<void> {
    if (this.#blocked) throw new ConnectError('Room access was revoked', Code.PermissionDenied);
    const epoch = this.accessEpoch;
    this.#pendingWrites++;
    try {
      const notice = await this.#api.clear(this.roomId, expectedVersion);
      this.#acceptWrite(notice, epoch);
    } finally {
      this.#pendingWrites--;
    }
  }

  /** A follow-up read could reach a stale replica. Fence pending reads instead. */
  #acceptWrite(notice: RoomNotice, epoch: number): void {
    if (this.#blocked || this.accessEpoch !== epoch) return;
    this.#generation++;
    this.#request?.abort();
    batch(() => {
      this.#setNotice(notice);
      this.#error.set(null);
      this.#supported.set(true);
    });
    // A live invalidation may cover a later write. Keep its cursor fence when
    // replacing the pending read; never replace it with an unfenced refresh.
    if (this.#minimumCursor && this.#retained) void this.refresh();
  }

  /**
   * Reconciles a realtime reset without discarding the displayed notice or drafts.
   * Only access loss clears private content and advances the draft epoch. A
   * reconnect cannot unblock a revoked store; restoreAccess owns that boundary.
   */
  reset(accessRevoked = false): void {
    this.#generation++;
    this.#request?.abort();
    this.#minimumCursor = undefined;
    if (!accessRevoked) {
      if (this.#retained) void this.refresh();
      return;
    }
    this.#blocked = true;
    batch(() => {
      this.#accessEpoch.set(this.#accessEpoch.get() + 1);
      this.#setNotice(null);
      this.#error.set(null);
    });
  }

  /** Reopens a store only after the registry confirms membership again. */
  restoreAccess(): void {
    if (!this.#blocked) return;
    this.#blocked = false;
    if (this.#retained) void this.refresh();
  }

  dispose(): void {
    this.reset(true);
    this.#retained = 0;
    this.#changes.clear();
  }

  #setNotice(notice: RoomNotice | null): void {
    this.#notice.set(notice);
    this.#changes.emit(notice);
  }
}
