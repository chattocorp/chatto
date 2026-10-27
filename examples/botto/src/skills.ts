/** Give Pi a text reader confined to the operator-owned skills directory. No execution or writes. */
import { constants } from 'node:fs';
import { open, realpath } from 'node:fs/promises';
import { isAbsolute, relative, resolve, sep } from 'node:path';
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';

const MAX_FILE_BYTES = 256 * 1024;
const MAX_OUTPUT_CHARS = 20_000;

/** Test path components, not a string prefix (skills-other is outside skills). */
function contained(root: string, path: string): boolean {
  const child = relative(root, path);
  return child !== '..' && !child.startsWith(`..${sep}`) && !isAbsolute(child);
}

/** Read a regular UTF-8 file after resolving symlinks. The directory is operator-controlled. */
async function readSkillFile(directory: string, path: string, signal?: AbortSignal) {
  signal?.throwIfAborted();
  const root = await realpath(directory);
  const requested = resolve(directory, path);
  // Reject lexical escapes before filesystem access, then reject resolved symlink escapes.
  if (!contained(resolve(directory), requested)) throw new Error('Outside skills directory');
  const resolved = await realpath(requested);
  if (!contained(root, resolved)) throw new Error('Outside skills directory');
  const file = await open(
    resolved,
    constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK
  );
  try {
    const stat = await file.stat();
    if (!stat.isFile() || stat.size > MAX_FILE_BYTES) throw new Error('Not a small regular file');
    // Recheck the path after opening; the agent cannot modify the tree itself.
    if ((await realpath(requested)) !== resolved) throw new Error('Skill changed during read');
    const bytes = Buffer.alloc(MAX_FILE_BYTES + 1);
    let length = 0;
    while (length < bytes.length) {
      signal?.throwIfAborted();
      const { bytesRead } = await file.read(bytes, length, bytes.length - length, null);
      if (!bytesRead) break;
      length += bytesRead;
    }
    if (length > MAX_FILE_BYTES) throw new Error('Skill file is too large');
    const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, length));
    if (text.includes('\0')) throw new Error('Not a text file');
    signal?.throwIfAborted();
    return text;
  } finally {
    await file.close();
  }
}

/** Use Pi's conventional tool name so its native skill catalog can advertise instructions. */
export function skillReadExtension(directory: string): ExtensionFactory {
  return (pi) => {
    pi.registerTool({
      name: 'read',
      label: 'Read skill file',
      description:
        'Read a UTF-8 skill or supporting document inside the skills directory only. Use the absolute path from the skill catalog, or a path relative to the skills directory. Cannot read other files, execute scripts, or write. Files are limited to 256 KiB; output is paginated by line.',
      parameters: Type.Object({
        path: Type.String({ minLength: 1 }),
        offset: Type.Optional(Type.Integer({ minimum: 1 })),
        limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 200 }))
      }),
      async execute(_id, { path, offset = 1, limit = 200 }, signal) {
        try {
          const text = await readSkillFile(directory, path, signal);
          const lines = text.split('\n');
          const selected: string[] = [];
          let size = 0;
          for (const line of lines.slice(offset - 1, offset - 1 + limit)) {
            if (line.length > MAX_OUTPUT_CHARS) throw new Error('Line is too long');
            const added = line.length + (selected.length ? 1 : 0);
            if (size + added > MAX_OUTPUT_CHARS) break;
            selected.push(line);
            size += added;
          }
          const next = offset + selected.length;
          return {
            content: [
              {
                type: 'text',
                text:
                  selected.join('\n') +
                  (next <= lines.length ? `\n[Continue with offset=${next}.]` : '')
              }
            ],
            details: {}
          };
        } catch {
          signal?.throwIfAborted();
          // Do not expose host paths, file contents, or OS errors on rejection.
          throw new Error(
            'Cannot read this skill file. Use a regular UTF-8 file inside the skills directory (maximum 256 KiB and 20,000 characters per line).'
          );
        }
      }
    });
  };
}
