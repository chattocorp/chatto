/** A private temporary directory for the bot process. Pi's codemode writes the complete output
 * of a long script to the temporary directory and never deletes it. That output can contain
 * thread messages and GitHub content, so it must not land in a shared, world-readable `/tmp`. */
import { chmod, lstat, mkdir, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

let active: string | undefined;

/**
 * Point `TMPDIR` at `<temporary directory>/chattobot-<uid>`, readable only by the bot's user, and
 * empty it. Node reads `TMPDIR` whenever code asks for the temporary directory, so later temporary
 * files of this process, including Pi's, go there. Later calls in the same process return the
 * same directory without emptying it again. Throws when the path exists but is not a directory
 * that the bot's user owns, for example a symbolic link that another user created.
 */
export async function usePrivateTempDirectory(base = tmpdir()): Promise<string> {
  if (active) return active;
  const uid = process.getuid?.();
  const directory = join(base, `chattobot-${uid ?? 'user'}`);
  await mkdir(directory, { mode: 0o700 }).catch((error: NodeJS.ErrnoException) => {
    if (error.code !== 'EEXIST') throw error;
  });
  const info = await lstat(directory);
  if (!info.isDirectory() || (uid !== undefined && info.uid !== uid))
    throw new Error(`${directory} is not a private directory of the bot's user`);
  await chmod(directory, 0o700);
  // Output of earlier runs is not needed after a restart.
  for (const entry of await readdir(directory))
    await rm(join(directory, entry), { recursive: true, force: true });
  process.env.TMPDIR = directory;
  active = directory;
  return directory;
}
