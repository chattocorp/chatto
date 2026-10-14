/** A read-only `git` tool for the investigator. It runs selected history commands in the
 * investigated directory, without a shell and without the host's Git configuration. */
import { execFile } from 'node:child_process';
import { defineAgentExtension, Type } from 'runling';

/** Subcommands that only read the repository. The first argument must be one of them, so
 * global options such as `-c` or `-C` cannot come before it. */
const READ_COMMANDS = new Set([
  'log',
  'show',
  'blame',
  'diff',
  'grep',
  'ls-files',
  'ls-tree',
  'rev-parse',
  'shortlog',
  'describe'
]);

/** Options that write files, run programs, or read files outside the repository. Each one is
 * also denied in its `--option=value` form; `-O` also with an attached value. */
const DENIED_OPTIONS = [
  '--output',
  '--ext-diff',
  '--textconv',
  '--open-files-in-pager',
  '--no-index'
];

/** Check one git command line. Returns why it is not allowed, or undefined. */
export function gitPolicy(args: readonly string[]): string | undefined {
  const [command, ...options] = args;
  if (!command || !READ_COMMANDS.has(command))
    return `Only these git commands are available: ${[...READ_COMMANDS].join(', ')}.`;
  for (const option of options) {
    const name = option.split('=', 1)[0]!;
    if (DENIED_OPTIONS.includes(name) || /^-O/.test(option))
      return `The option ${name} is not available.`;
  }
  return undefined;
}

const MAX_OUTPUT = 40_000;

/** Run a checked git command in `directory`. Output is bounded. */
export function runGit(directory: string, args: readonly string[], signal?: AbortSignal) {
  return new Promise<string>((resolve, reject) => {
    execFile(
      'git',
      ['--no-pager', '-c', 'core.hooksPath=/dev/null', '-c', 'diff.external=', ...args],
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
            ? `${stdout.slice(0, MAX_OUTPUT)}\n[… ${stdout.length - MAX_OUTPUT} characters omitted. Narrow the command, for example with -n, a path, or -L.]`
            : stdout
        );
      }
    );
  });
}

/** The investigator's `git` tool for `directory`. */
export function gitExtension(directory: string) {
  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'git',
      label: 'Read Git history',
      description: `Run a read-only git command in the repository: ${[...READ_COMMANDS].join(', ')}. Use it to see who changed code, when, and why, for example ["log", "-n", "10", "--", "path"], ["blame", "-L", "40,60", "path"], or ["show", "<commit>"].`,
      parameters: Type.Object({
        args: Type.Array(Type.String({ maxLength: 1000 }), {
          minItems: 1,
          maxItems: 40,
          description: 'git arguments without the program name'
        })
      }),
      async execute(_id, { args }, signal) {
        const problem = gitPolicy(args);
        if (problem) throw new Error(problem);
        const output = await runGit(directory, args, signal);
        return { content: [{ type: 'text', text: output || '(no output)' }], details: {} };
      }
    });
  });
}
