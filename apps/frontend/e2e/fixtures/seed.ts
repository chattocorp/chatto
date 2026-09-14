import type { APIRequestContext } from '@playwright/test';

/** Reproducible workload shape. Message count includes thread replies. */
export interface SeedOptions {
  seed: number;
  users: number;
  rooms: number;
  messages: number;
  threadReplies?: number;
}

/** IDs and generated content in creation order, ready for API and browser reads. */
export interface SeedResult {
  version: string;
  seed: number;
  users: { id: string; login: string; displayName: string }[];
  rooms: { id: string; name: string }[];
  messages: {
    id: string;
    roomId: string;
    authorId: string;
    body: string;
    threadRootId?: string;
  }[];
}

/**
 * Seed a test-endpoint build through normal domain operations. Does not retry:
 * a failure can leave partial data. Each call adds another dataset.
 * Generated accounts have no password; use loginSeededUser for browser viewers.
 */
export async function seedData(
  request: APIRequestContext,
  options: SeedOptions,
  timeout = 60_000
): Promise<SeedResult> {
  if (!Number.isSafeInteger(options.seed)) throw new Error('Seed must be a safe integer');
  const response = await request.post('/auth/test/seed', { data: options, timeout });
  if (!response.ok()) {
    throw new Error(`Data seeding failed (${response.status()}); partial data may remain`);
  }
  return (await response.json()) as SeedResult;
}

/**
 * Give this request/browser context a real session for a seeded account without
 * password hashing. Call before loading the app, with a fresh browser context
 * for each independent viewer. Available only on test-endpoint builds.
 */
export async function loginSeededUser(
  request: APIRequestContext,
  user: Pick<SeedResult['users'][number], 'id'>
): Promise<void> {
  const response = await request.post('/auth/test/create-session', { data: { userId: user.id } });
  if (!response.ok()) throw new Error(`Seeded user session failed (${response.status()})`);
}
