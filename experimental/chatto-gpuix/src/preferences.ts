/** Private, non-secret native settings. Credentials and message content are never written. */
import { mkdirSync, readFileSync, renameSync, writeFileSync, chmodSync, lstatSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

export interface Preferences {
  origin: string;
  login: string;
  room: string;
}
function directory(): string {
  return (
    process.env.CHATTO_GPUIX_CONFIG_DIR ??
    join(homedir(), 'Library', 'Application Support', 'Chatto GPUix')
  );
}
export function readPreferences(): Preferences {
  const fallback = {
    origin: `http://127.0.0.1:${process.env.CHATTO_DEV_CHATTO_PORT ?? '4000'}`,
    login: '',
    room: ''
  };
  try {
    const file = join(directory(), 'settings.json');
    const meta = lstatSync(file);
    if (meta.isSymbolicLink() || meta.size > 64 * 1024) return fallback;
    const saved: unknown = JSON.parse(readFileSync(file, 'utf8'));
    if (!saved || typeof saved !== 'object') return fallback;
    const value = saved as Record<string, unknown>;
    return {
      origin: typeof value.origin === 'string' ? value.origin : fallback.origin,
      login: typeof value.login === 'string' ? value.login : '',
      room: typeof value.room === 'string' ? value.room : ''
    };
  } catch {
    return fallback;
  }
}
export function savePreferences(value: Preferences): void {
  const dir = directory();
  try {
    if (lstatSync(dir).isSymbolicLink()) throw new Error('Invalid settings directory');
  } catch (error) {
    if (!(error instanceof Error && 'code' in error && error.code === 'ENOENT'))
      throw new Error('Could not save connection settings');
  }
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  chmodSync(dir, 0o700);
  const temporary = join(dir, `.settings-${crypto.randomUUID()}`);
  writeFileSync(temporary, JSON.stringify(value), { mode: 0o600, flag: 'wx' });
  renameSync(temporary, join(dir, 'settings.json'));
}
