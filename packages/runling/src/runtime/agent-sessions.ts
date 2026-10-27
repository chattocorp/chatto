/** Stable agent session files owned by a recorded run and its retention policy. */
import { createHash } from 'node:crypto';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import type { RunIdentity } from './context.ts';

/** Get a private transcript path. Omit the key for a fresh session on each call. Reusing a key
 * reopens that session after recovery. Unrecorded hosts return undefined (in-memory sessions).
 * Runling removes these files when it purges the closed run. */
export async function agentSessionFile(
  ctx: { run?: RunIdentity },
  key: string = crypto.randomUUID()
): Promise<string | undefined> {
  if (!key.trim()) throw new TypeError('Agent session key must not be empty');
  if (!ctx.run?.filesDirectory) return undefined;
  const directory = resolve(ctx.run.filesDirectory, 'agents');
  await mkdir(directory, { recursive: true, mode: 0o700 });
  return resolve(directory, `${createHash('sha256').update(key).digest('hex')}.jsonl`);
}
