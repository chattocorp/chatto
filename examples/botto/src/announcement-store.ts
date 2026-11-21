/** Durable announcement receipts. One Botto process owns each ledger. */
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

export interface PushSummary {
  before: string;
  after: string;
  commits: Array<{ id: string; message: string }>;
  truncated: boolean;
}
/** Bounded release evidence from a signed GitHub webhook. */
export interface ReleaseSummary {
  kind: 'release';
  id: number;
  tag: string;
  name: string;
  body: string;
  prerelease: boolean;
  publishedAt: string;
  truncated: boolean;
}

/** Pushes retain their original shape so existing receipts remain readable. */
export type AnnouncementEvent = PushSummary | ReleaseSummary;
export type Announcement = AnnouncementReceipt &
  ({ push: PushSummary; release?: never } | { release: ReleaseSummary; push?: never });

interface AnnouncementReceipt {
  deliveryId: string;
  status: 'queued' | 'sending' | 'sent' | 'skipped';
  /** Stable caller UUID and exact body are persisted before the external write. */
  idempotencyKey: string;
  body?: string;
  messageId?: string;
  /** Separate durable receipt for details, sent only after the root receipt is saved. */
  detailsBody?: string;
  detailsIdempotencyKey?: string;
  detailsStatus?: 'queued' | 'sending' | 'sent';
  detailsMessageId?: string;
}

export class AnnouncementStore {
  private readonly path: string;
  private records: Record<string, Announcement>;
  constructor(directory: string, identity: readonly string[]) {
    mkdirSync(directory, { recursive: true, mode: 0o700 });
    this.path = join(
      directory,
      `announcements-${createHash('sha256').update(JSON.stringify(identity)).digest('hex')}.json`
    );
    try {
      const state = JSON.parse(readFileSync(this.path, 'utf8'));
      if (
        state.version !== 1 ||
        !state.records ||
        typeof state.records !== 'object' ||
        Array.isArray(state.records)
      )
        throw new Error();
      for (const [key, entry] of Object.entries(state.records)) {
        const r = entry as Announcement;
        if (
          !/^[a-f0-9]{64}$/.test(key) ||
          (!r.push && !r.release) ||
          (r.push !== undefined &&
            (r.release !== undefined ||
              typeof r.push.after !== 'string' ||
              !Array.isArray(r.push.commits))) ||
          (r.release !== undefined &&
            (r.release.kind !== 'release' ||
              !Number.isSafeInteger(r.release.id) ||
              r.release.id <= 0 ||
              typeof r.release.tag !== 'string' ||
              typeof r.release.body !== 'string' ||
              typeof r.release.name !== 'string' ||
              typeof r.release.prerelease !== 'boolean' ||
              typeof r.release.publishedAt !== 'string' ||
              typeof r.release.truncated !== 'boolean')) ||
          typeof r.idempotencyKey !== 'string' ||
          !['queued', 'sending', 'sent', 'skipped'].includes(r.status) ||
          ((r.status === 'sending' || r.status === 'sent') && typeof r.body !== 'string') ||
          (r.status === 'skipped' && (r.body !== undefined || r.detailsStatus !== undefined)) ||
          (r.detailsStatus !== undefined &&
            (!['queued', 'sending', 'sent'].includes(r.detailsStatus) ||
              typeof r.detailsBody !== 'string' ||
              !r.detailsBody ||
              typeof r.detailsIdempotencyKey !== 'string' ||
              (r.status === 'sent' && !r.messageId) ||
              (r.detailsStatus !== 'queued' && r.status !== 'sent') ||
              (r.detailsStatus === 'sent' && !r.detailsMessageId)))
        )
          throw new Error();
      }
      this.records = state.records;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT')
        throw new Error('Invalid announcement ledger; restore it before enabling announcements.');
      this.records = {};
      this.save(this.records);
    }
  }
  /** Deduplicate signed push boundaries or release ID and stage, never delivery headers. */
  accept(event: AnnouncementEvent, deliveryId: string): boolean {
    const release = 'kind' in event;
    const key = createHash('sha256')
      .update(
        JSON.stringify(
          release ? ['release', event.id, event.prerelease] : [event.before, event.after]
        )
      )
      .digest('hex');
    if (this.records[key]) return false;
    if (this.queued.length >= 1000) throw new Error('Announcement queue full');
    this.save({
      ...this.records,
      [key]: {
        ...(release ? { release: event } : { push: event }),
        deliveryId,
        status: 'queued',
        idempotencyKey: randomUUID()
      }
    });
    return true;
  }
  /** Confirmed roots include legacy receipts and roots whose details need operator review. */
  get sentRoots(): string[] {
    return Object.values(this.records)
      .filter((record) => record.status === 'sent' && record.messageId)
      .map((record) => record.messageId!);
  }
  get queued(): string[] {
    return Object.keys(this.records).filter((key) => {
      const r = this.records[key]!;
      return r.status === 'queued' || (r.status === 'sent' && r.detailsStatus === 'queued');
    });
  }
  get uncertain(): number {
    return Object.values(this.records).filter(
      (r) => r.status === 'sending' || r.detailsStatus === 'sending'
    ).length;
  }
  get(key: string): Announcement | undefined {
    return this.records[key];
  }
  /** Retain a terminal no-post decision so replay and restart cannot requeue it. */
  skip(key: string): void {
    const record = this.records[key];
    if (!record || record.status !== 'queued' || record.body !== undefined)
      throw new Error('Announcement cannot be skipped after preparation.');
    this.save({ ...this.records, [key]: { ...record, status: 'skipped' } });
  }
  prepare(key: string, body: string, details?: string): void {
    const record = this.records[key];
    if (!record || record.status !== 'queued') throw new Error('Announcement is not queued');
    this.save({
      ...this.records,
      [key]: {
        ...record,
        body,
        ...(details
          ? {
              detailsBody: details,
              detailsIdempotencyKey: randomUUID(),
              detailsStatus: 'queued' as const
            }
          : {})
      }
    });
  }
  sending(key: string): void {
    const record = this.records[key];
    if (!record || record.status !== 'queued' || !record.body)
      throw new Error('Announcement is not prepared');
    this.save({ ...this.records, [key]: { ...record, status: 'sending' } });
  }
  complete(key: string, messageId: string): void {
    const record = this.records[key];
    if (!record || record.status !== 'sending' || !messageId)
      throw new Error('Announcement is not sending');
    this.save({ ...this.records, [key]: { ...record, status: 'sent', messageId } });
  }
  /** Persist reply intent before the write; unknown results must not retry automatically. */
  sendingDetails(key: string): void {
    const record = this.records[key];
    if (
      !record ||
      record.status !== 'sent' ||
      !record.messageId ||
      record.detailsStatus !== 'queued'
    )
      throw new Error('Announcement details are not ready.');
    this.save({ ...this.records, [key]: { ...record, detailsStatus: 'sending' } });
  }
  /** Save the reply receipt independently of the root receipt. */
  completeDetails(key: string, messageId: string): void {
    const record = this.records[key];
    if (!record || record.detailsStatus !== 'sending' || !messageId)
      throw new Error('Announcement details are not sending.');
    this.save({
      ...this.records,
      [key]: { ...record, detailsStatus: 'sent', detailsMessageId: messageId }
    });
  }
  private save(records: Record<string, Announcement>): void {
    writeFileSync(`${this.path}.new`, JSON.stringify({ version: 1, records }), {
      mode: 0o600,
      flush: true
    });
    renameSync(`${this.path}.new`, this.path);
    this.records = records;
  }
}
