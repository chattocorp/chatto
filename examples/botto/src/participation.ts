/** Share durable host-opened thread participation across independent Botto sources. */
import { createHash } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { stat } from 'node:fs/promises';
import { join } from 'node:path';

function marker(directory: string, identity: readonly string[]): string {
  return join(
    directory,
    'participation',
    createHash('sha256').update(JSON.stringify(identity)).digest('hex') + '.active'
  );
}

/** Record a confirmed thread root without creating a synthetic agent transcript. */
export function rememberParticipation(directory: string, identity: readonly string[]): void {
  mkdirSync(join(directory, 'participation'), { recursive: true, mode: 0o700 });
  try {
    writeFileSync(marker(directory, identity), '', { flag: 'wx', mode: 0o600, flush: true });
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error;
  }
}

/** Read without caching so another source's newly registered roots are visible immediately. */
export async function hasParticipation(
  directory: string,
  identity: readonly string[]
): Promise<boolean> {
  try {
    return (await stat(marker(directory, identity))).isFile();
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return false;
    throw error;
  }
}
