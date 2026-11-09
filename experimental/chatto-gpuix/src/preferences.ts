/** Read legacy non-secret form preferences. New sessions and UI state use the encrypted vault. */
import { readFileSync, lstatSync } from 'node:fs';
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
