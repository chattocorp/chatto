/** Read-only Git history tools for the investigator. The host builds each command line; the
 * agent supplies only checked values: revisions, relative paths, numbers, and search text.
 * So the agent cannot add options that run programs, write files, or read files outside the
 * repository. Commands run without a shell and without the host's Git configuration. */
import { execFile } from 'node:child_process';
import { defineAgentExtension, Type } from 'runling';

const MAX_OUTPUT = 40_000;

/** A revision such as `HEAD~3`, `abc123`, `main`, or a range `a..b`. Never an option. */
export function checkRevision(value: string): string {
  if (!/^[A-Za-z0-9][A-Za-z0-9._/^~@{}-]{0,199}$/.test(value) || value.includes('@{-'))
    throw new Error(`${JSON.stringify(value)} is not a valid revision`);
  return value;
}

/** A path relative to the repository root, inside it. Never an option. */
export function checkPath(value: string): string {
  const segments = value.split('/');
  if (
    !value ||
    value.length > 1000 ||
    value.startsWith('-') ||
    value.startsWith('/') ||
    value.includes('\\') ||
    value.includes(':') ||
    segments.includes('..') ||
    /[\x00-\x1f]/.test(value)
  )
    throw new Error(`${JSON.stringify(value)} is not a relative path inside the repository`);
  return value;
}

/** Run git in `directory`. Output is bounded. */
export function runGit(directory: string, args: readonly string[], signal?: AbortSignal) {
  return new Promise<string>((resolve, reject) => {
    execFile(
      'git',
      ['--no-pager', '-c', 'core.hooksPath=/dev/null', ...args],
      {
        cwd: directory,
        env: {
          PATH: process.env.PATH ?? '/usr/bin:/bin',
          HOME: directory,
          // Ignore the host's system and global Git configuration, such as aliases and pagers.
          GIT_CONFIG_NOSYSTEM: '1',
          GIT_CONFIG_GLOBAL: '/dev/null',
          GIT_TERMINAL_PROMPT: '0',
          GIT_OPTIONAL_LOCKS: '0'
        },
        signal,
        timeout: 30_000,
        maxBuffer: 16 * 1024 * 1024
      },
      (error, stdout, stderr) => {
        if (error && signal?.aborted) return reject(signal.reason);
        if (error) return reject(new Error((stderr || error.message).slice(0, 2000)));
        resolve(
          stdout.length > MAX_OUTPUT
            ? `${stdout.slice(0, MAX_OUTPUT)}\n[… ${stdout.length - MAX_OUTPUT} characters omitted. Narrow the request.]`
            : stdout
        );
      }
    );
  });
}

/** Options that keep diffs from running external programs. */
const SAFE_DIFF = ['--no-ext-diff', '--no-textconv'];

/** The investigator's Git history tools for `directory`. */
export function gitExtension(directory: string) {
  const text = (output: string) => ({
    content: [{ type: 'text' as const, text: output || '(no output)' }],
    details: {}
  });
  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'gitLog',
      label: 'Read Git log',
      description:
        'List commits, newest first, with their date, author, and subject. Filter by path, by a revision or range, or by commits that add or remove a text (pickaxe search).',
      parameters: Type.Object({
        path: Type.Optional(Type.String({ description: 'Only commits that change this path' })),
        revision: Type.Optional(
          Type.String({ description: 'A revision or range, such as main~50..main' })
        ),
        added: Type.Optional(
          Type.String({ maxLength: 200, description: 'Only commits that add or remove this text' })
        ),
        count: Type.Optional(Type.Integer({ minimum: 1, maximum: 100, description: 'Default 20' }))
      }),
      async execute(_id, { path, revision, added, count = 20 }, signal) {
        return text(
          await runGit(
            directory,
            [
              'log',
              `--max-count=${count}`,
              '--format=%h %ad %an%n    %s',
              '--date=short',
              ...(added !== undefined ? [`-S${added}`] : []),
              ...(revision ? [checkRevision(revision)] : []),
              '--',
              ...(path ? [checkPath(path)] : [])
            ],
            signal
          )
        );
      }
    });
    pi.registerTool({
      name: 'gitBlame',
      label: 'Read Git blame',
      description: 'Show the commit, author, and date that last changed each line of a file range.',
      parameters: Type.Object({
        path: Type.String(),
        start: Type.Integer({ minimum: 1 }),
        end: Type.Integer({ minimum: 1 }),
        revision: Type.Optional(Type.String({ description: 'Blame the file at this revision' }))
      }),
      async execute(_id, { path, start, end, revision }, signal) {
        if (end < start || end - start > 500) throw new Error('Blame at most 500 lines at once');
        return text(
          await runGit(
            directory,
            [
              'blame',
              '--date=short',
              `-L${start},${end}`,
              ...(revision ? [checkRevision(revision)] : []),
              '--',
              checkPath(path)
            ],
            signal
          )
        );
      }
    });
    pi.registerTool({
      name: 'gitShow',
      label: 'Read Git commit',
      description:
        'Show a commit: its message and its changes, optionally only for one path. Use it to see why and how code changed.',
      parameters: Type.Object({
        revision: Type.String(),
        path: Type.Optional(Type.String({ description: 'Only the changes to this path' })),
        stat: Type.Optional(
          Type.Boolean({ description: 'Only list the changed files, without the changes' })
        )
      }),
      async execute(_id, { revision, path, stat }, signal) {
        return text(
          await runGit(
            directory,
            [
              'show',
              ...SAFE_DIFF,
              '--date=short',
              ...(stat ? ['--stat'] : []),
              checkRevision(revision),
              '--',
              ...(path ? [checkPath(path)] : [])
            ],
            signal
          )
        );
      }
    });
  });
}

/** The names of the Git history tools. */
export const GIT_TOOLS = ['gitLog', 'gitBlame', 'gitShow'] as const;
