/** A private temporary directory for the bot process. Pi's codemode writes the complete output
 * of a long script to the temporary directory. That output can contain thread messages and
 * GitHub content, so it must not land in a shared, world-readable `/tmp`. */
import { rmSync } from 'node:fs';
import { mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Runling reloads configuration modules without a cache, so the state lives on globalThis.
const STATE = Symbol.for('chattobot.privateTemp');
type State = { [STATE]?: Promise<string> };

/**
 * Create a directory that only the bot's user can read, with a random name, and point `TMPDIR`
 * at it. Node reads `TMPDIR` whenever code asks for the temporary directory on POSIX systems, so
 * later temporary files of this process, including Pi's, go there. The process removes the
 * directory when it exits; after a crash, the private directory stays. Later calls in the same
 * process, also after a configuration reload, return the same directory.
 */
export function usePrivateTempDirectory(): Promise<string> {
  const state = globalThis as State;
  state[STATE] ??= mkdtemp(join(tmpdir(), 'chattobot-')).then((directory) => {
    process.env.TMPDIR = directory;
    process.once('exit', () => rmSync(directory, { recursive: true, force: true }));
    return directory;
  });
  return state[STATE];
}
