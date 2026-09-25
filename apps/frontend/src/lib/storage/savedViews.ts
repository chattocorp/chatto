// SPDX-License-Identifier: Apache-2.0

import type { TimelineEventView } from '$lib/render/timelineEvents';
import type { ServerPresentationSnapshot } from './presentationSnapshot';

/** A normal room resource and its bounded presentation state. */
export type SavedRoom = {
  id: string;
  name: string;
  kind?: number;
  universal?: boolean;
  resource: string;
  events: TimelineEventView[];
  hasReachedStart?: boolean;
  /** Absent when this room's timeline has never been loaded or was evicted. */
  timeline?: { startCursor?: string; endCursor?: string; hasNewer: boolean };
  members?: { ids: string[]; totalCount: number; complete: boolean; presence: [string, number][] };
  threads?: {
    rootId: string;
    events: TimelineEventView[];
    hasReachedStart: boolean;
    timeline: { startCursor?: string; endCursor?: string; hasNewer: boolean };
  }[];
};

export type SavedView = {
  version: 3;
  /** Opaque, viewer-bound server sequence token, committed with all resource records. */
  checkpoint: string;
  /** Time the reconciliation barrier accepted this checkpoint, not the time of a disk write. */
  checkpointAt: number;
  serverId: string;
  userId: string;
  viewerName?: string;
  serverName: string;
  savedAt: number;
  rooms: SavedRoom[];
  presentation: ServerPresentationSnapshot;
};

/** Synchronous privacy fence shared with the lazily loaded disk implementation. */
export const snapshotStorageGeneration = { value: 0 };

let boundaryMillisecond = 0;
let boundaryOrdinal = 0;

/** Order privacy requests and accepted checkpoints even within one clock tick. */
export function snapshotBoundaryTime(): number {
  const now = Date.now();
  boundaryOrdinal = now === boundaryMillisecond ? boundaryOrdinal + 1 : 0;
  boundaryMillisecond = now;
  return now + boundaryOrdinal / 1000;
}

/** Fence queued reads/writes immediately, including before the storage chunk loads. */
export function invalidateSavedViewWrites(): void {
  snapshotStorageGeneration.value++;
}

/** Load a complete compatible resource set for the exact server and viewer. */
export async function loadSavedView(
  serverId: string,
  userId: string | null
): Promise<SavedView | null> {
  if (!userId) return null;
  const generation = snapshotStorageGeneration.value;
  try {
    const storage = await import('./projectionSnapshotStorage');
    return await storage.loadSavedView(serverId, userId, generation);
  } catch {
    return null;
  }
}

/** Persist a complete checkpoint set; writes coalesce without a route-lifetime timer. */
export async function saveView(view: SavedView): Promise<void> {
  const generation = snapshotStorageGeneration.value;
  try {
    const storage = await import('./projectionSnapshotStorage');
    await storage.saveView(view, generation);
  } catch {
    /* A missing offline chunk must not interrupt live state. */
  }
}

/** Remove private snapshots and fence older writes before loading storage code. */
export async function clearSavedView(serverId: string, userId?: string): Promise<void> {
  const cutoff = snapshotBoundaryTime();
  invalidateSavedViewWrites();
  try {
    const storage = await import('./projectionSnapshotStorage');
    await storage.clearSavedView(serverId, userId, cutoff);
  } catch {
    /* Storage can be unavailable; the synchronous write fence remains. */
  }
}

/** Remove all device snapshots, including records from other server accounts. */
export async function clearAllSavedViews(): Promise<void> {
  const cutoff = snapshotBoundaryTime();
  invalidateSavedViewWrites();
  try {
    const storage = await import('./projectionSnapshotStorage');
    await storage.clearAllSavedViews(cutoff);
  } catch {
    /* Storage can be unavailable; the synchronous write fence remains. */
  }
}
