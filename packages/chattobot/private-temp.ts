/** A private temporary directory for the bot process. Pi's codemode writes the complete output
 * of a long script to the temporary directory. That output can contain thread messages and
 * GitHub content, so it must not land in a shared, world-readable `/tmp`. */
import { rmSync } from 'node:fs';
import { lstat, mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Runling reloads configuration modules without a cache, so the state lives on globalThis.
const STATE = Symbol.for('chattobot.privateTemp');
const BASE = Symbol.for('chattobot.privateTempBase');
type State = { [STATE]?: Promise<string | undefined>; [BASE]?: string };

/** True when `path` is a directory that only this user can read. Windows has no such modes. */
async function isPrivate(path: string): Promise<boolean> {
  try {
    const info = await lstat(path);
    if (!info.isDirectory()) return false;
    if (process.platform === 'win32') return true;
    return info.uid === process.getuid?.() && (info.mode & 0o777) === 0o700;
  } catch {
    return false;
  }
}

/**
 * Create a directory that only the bot's user can read, with a random name, and point `TMPDIR`
 * at it. Node reads `TMPDIR` whenever code asks for the temporary directory on POSIX systems, so
 * later temporary files of this process, including Pi's, go there. Later calls in the same
 * process, also after a configuration reload, return the same directory while it stays private.
 * A system cleaner can delete an idle temporary directory, so callers check it again at each
 * conversation, and a missing or changed directory is replaced. The process removes the
 * directory when it exits; after a crash, the private directory stays.
 */
export function usePrivateTempDirectory(): Promise<string> {
  const state = globalThis as State;
  // The system temporary directory, read before TMPDIR points at a private directory. A failed
  // replacement must not lose it, or later attempts would use the deleted directory as base.
  state[BASE] ??= tmpdir();
  const base = state[BASE];
  const next = (state[STATE] ?? Promise.resolve(undefined))
    .catch(() => undefined)
    .then(async (current) => {
      if (current && (await isPrivate(current))) return current;
      const path = await mkdtemp(join(base, 'chattobot-'));
      process.env.TMPDIR = path;
      process.once('exit', () => {
        // A child process can still write there; a failed cleanup must not change the exit.
        try {
          rmSync(path, { recursive: true, force: true });
        } catch {
          // The directory stays private.
        }
      });
      return path;
    });
  state[STATE] = next;
  return next as Promise<string>;
}
