/** Run gh in an isolated environment. The process inherits no host environment except PATH and
 * TMPDIR, so it cannot see the host's gh login, aliases, extensions, other credentials, or the
 * App private key. It runs without a shell, in a new temporary directory that holds only its
 * configuration. The token is in that configuration, not in the environment, so jq and template
 * expressions cannot read it. */
import { execFile } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

/** The result of one gh command. `output` is bounded and redacted. */
export interface GhResult {
  ok: boolean;
  output: string;
}

/** Run one gh command with an installation token for `repository`. Injectable for tests. */
export type GhRunner = (
  args: readonly string[],
  options: {
    token: string;
    repository: string;
    signal: AbortSignal;
    timeoutMs?: number;
    /** Characters of output to keep. Defaults to about 32,000: the start and the end. */
    limit?: number;
  }
) => Promise<GhResult>;

/** Output kept from one command: the start of long lists and the end of long logs. */
const HEAD = 8_000;
const TAIL = 24_000;

/** Remove tokens and email addresses from gh output, and bound its length. */
export function redactGhOutput(text: string, token?: string, limit = HEAD + TAIL): string {
  // Real tokens are long; a short value would redact ordinary words.
  let result = token && token.length >= 8 ? text.split(token).join('[token]') : text;
  result = result
    .replace(/\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b/g, '[token]')
    .replace(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi, '[email]')
    .replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '');
  const head = Math.round((limit * HEAD) / (HEAD + TAIL));
  const tail = limit - head;
  return result.length > limit
    ? `${result.slice(0, head)}\n[… ${result.length - limit} characters omitted …]\n${result.slice(-tail)}`
    : result;
}

export const runGh: GhRunner = async (args, { token, repository, signal, timeoutMs, limit }) => {
  const home = await mkdtemp(join(tmpdir(), 'chattobot-gh-'));
  try {
    if (!/^[A-Za-z0-9_.-]+$/.test(token)) throw new Error('Unexpected token format');
    await writeFile(
      join(home, 'hosts.yml'),
      `github.com:\n    oauth_token: ${token}\n    git_protocol: https\n`,
      { mode: 0o600 }
    );
    return await new Promise<GhResult>((resolve, reject) => {
      const child = execFile(
        'gh',
        [...args],
        {
          cwd: home,
          env: {
            PATH: process.env.PATH ?? '/usr/bin:/bin',
            ...(process.env.TMPDIR ? { TMPDIR: process.env.TMPDIR } : {}),
            HOME: home,
            GH_CONFIG_DIR: home,
            XDG_CONFIG_HOME: home,
            XDG_CACHE_HOME: home,
            XDG_STATE_HOME: home,
            GH_REPO: repository,
            GH_HOST: 'github.com',
            GH_PROMPT_DISABLED: '1',
            GH_NO_UPDATE_NOTIFIER: '1',
            GH_NO_EXTENSION_UPDATE_NOTIFIER: '1',
            GH_SPINNER_DISABLED: '1',
            // gh telemetry would tell GitHub which commands the bot runs.
            GH_TELEMETRY: 'false',
            NO_COLOR: '1',
            GH_PAGER: 'cat',
            PAGER: 'cat'
          },
          signal,
          timeout: timeoutMs ?? 60_000,
          maxBuffer: 16 * 1024 * 1024,
          windowsHide: true
        },
        (error, stdout, stderr) => {
          if (error && signal.aborted) return reject(signal.reason);
          const text = [stdout, error ? stderr : ''].filter((part) => part.trim()).join('\n');
          resolve({
            ok: !error,
            output:
              redactGhOutput(text, token, limit).trim() ||
              (error
                ? (error as NodeJS.ErrnoException).code === 'ENOENT'
                  ? 'gh is not installed on the bot host.'
                  : error.killed
                    ? 'gh did not finish in time.'
                    : 'gh failed without output.'
                : '')
          });
        }
      );
      // gh gets no input: a command that would read standard input fails instead of waiting.
      child.stdin?.end();
    });
  } finally {
    await rm(home, { recursive: true, force: true });
  }
};
