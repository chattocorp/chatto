/** Confine the investigator's file tools to the repository's own files. Pi's `read`, `grep`,
 * `find`, and `ls` accept any path, also absolute ones, and `read` and a `grep` of one file
 * also read files that Git ignores. The investigator's answers reach chat users, so it must not
 * read host files, Git internals, or ignored files such as `.env` or private keys. */
import { realpath } from 'node:fs/promises';
import { dirname, isAbsolute, relative, resolve } from 'node:path';
import { defineAgentExtension } from 'runling';
import { runGit } from './git.ts';

/** The built-in file tools and their path parameter. */
const FILE_TOOLS = new Set(['read', 'grep', 'find', 'ls']);

/** The real path of `path`, or of its nearest existing ancestor joined with the rest. */
async function realPath(path: string): Promise<string> {
  try {
    return await realpath(path);
  } catch {
    const parent = dirname(path);
    if (parent === path) return path;
    return resolve(await realPath(parent), relative(parent, path));
  }
}

/** Why the investigator cannot use `path`, or undefined. `path` is relative to `directory` or
 * absolute. */
export async function confinementProblem(
  directory: string,
  path: string | undefined
): Promise<string | undefined> {
  if (path === undefined || path === '' || path === '.') return undefined;
  const root = await realpath(directory);
  const target = await realPath(resolve(root, path));
  const local = relative(root, target);
  if (local.startsWith('..') || isAbsolute(local))
    return `${path} is outside the repository. Use paths inside it.`;
  if (local.split(/[\\/]/).includes('.git')) return `${path} is inside .git. Use the git tools.`;
  // `check-ignore` exits with 0 when Git ignores the path, 1 when not, 128 outside a repository.
  const ignored = await runGit(root, ['check-ignore', '-q', '--', local]).then(
    () => true,
    () => false
  );
  if (ignored) return `${path} is ignored by Git and not part of the repository.`;
  return undefined;
}

export function confineExtension(directory: string) {
  return defineAgentExtension((pi) => {
    pi.on('tool_call', async (event) => {
      if (!FILE_TOOLS.has(event.toolName)) return;
      const input = event.input as { path?: unknown; pattern?: unknown; glob?: unknown };
      for (const value of [input.glob, event.toolName === 'find' ? input.pattern : undefined])
        if (typeof value === 'string' && value.split(/[\\/]/).includes('..'))
          return { block: true, reason: 'Patterns cannot leave the repository.' };
      const path = typeof input.path === 'string' ? input.path : undefined;
      const problem = await confinementProblem(directory, path);
      return problem ? { block: true, reason: problem } : undefined;
    });
  });
}
