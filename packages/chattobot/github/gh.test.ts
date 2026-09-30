import { chmod, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { redactGhOutput, runGh } from './gh.ts';

const folders: string[] = [];
afterEach(async () => {
  vi.unstubAllEnvs();
  await Promise.all(folders.splice(0).map((path) => rm(path, { recursive: true, force: true })));
});

/** Put a fake gh first on PATH. It prints its arguments, working directory, and environment. */
async function fakeGh(
  script = 'printf "%s\\n" "$@"; pwd; env | sort; cat "$GH_CONFIG_DIR/hosts.yml"'
) {
  const bin = await mkdtemp(join(tmpdir(), 'chattobot-fake-gh-'));
  folders.push(bin);
  await writeFile(join(bin, 'gh'), `#!/bin/sh\n${script}\n`);
  await chmod(join(bin, 'gh'), 0o755);
  vi.stubEnv('PATH', `${bin}:${process.env.PATH}`);
}

test('gh runs without host credentials, config, or a shell, in an empty directory', async () => {
  await fakeGh();
  vi.stubEnv('CHATTO_API_KEY', 'chatto-secret');
  vi.stubEnv('OPENROUTER_API_KEY', 'model-secret');
  vi.stubEnv('GITHUB_TOKEN', 'host-token');
  const result = await runGh(['issue', 'view', '1; echo injected'], {
    token: 'ghs_minted',
    repository: 'chattocorp/chatto',
    signal: AbortSignal.timeout(5000)
  });
  expect(result.ok).toBe(true);
  const lines = result.output.split('\n');
  // Arguments arrive unchanged, without shell interpretation.
  expect(lines.slice(0, 3)).toEqual(['issue', 'view', '1; echo injected']);
  expect(result.output).not.toContain('chatto-secret');
  expect(result.output).not.toContain('model-secret');
  expect(result.output).not.toContain('host-token');
  // The minted token is redacted from output too.
  expect(result.output).not.toContain('ghs_minted');
  // The token is in the gh configuration, not in the environment that jq could read.
  expect(lines.some((line) => line.startsWith('GH_TOKEN='))).toBe(false);
  expect(lines).toContain('    oauth_token: [token]');
  expect(lines).toContain('GH_REPO=chattocorp/chatto');
  expect(lines).toContain('GH_TELEMETRY=false');
  // HOME and the gh configuration are the empty working directory (pwd prints its real path).
  const home = lines.find((line) => line.startsWith('HOME='))!.slice('HOME='.length);
  expect(lines).toContain(`GH_CONFIG_DIR=${home}`);
  expect(lines[3]!.endsWith(home.split('/').at(-1)!)).toBe(true);
  expect(home).toContain('chattobot-gh-');
  const names = lines
    .filter((line) => /^[A-Za-z_][A-Za-z0-9_]*=/.test(line))
    .map((line) => line.split('=')[0]);
  expect(
    names.every((name) =>
      /^(GH_|XDG_|PATH$|HOME$|TMPDIR$|NO_COLOR$|PAGER$|PWD$|SHLVL$|_$|__CF|OLDPWD$)/.test(name!)
    )
  ).toBe(true);
});

test('a failed command returns its error output', async () => {
  await fakeGh('echo "issue not found" >&2; exit 1');
  const result = await runGh(['issue', 'view', '999'], {
    token: 't',
    repository: 'a/b',
    signal: AbortSignal.timeout(5000)
  });
  expect(result).toEqual({ ok: false, output: 'issue not found' });
});

test('redaction removes tokens and email addresses and bounds long output', () => {
  expect(
    redactGhOutput(
      'by dev@example.com with ghp_abcdefghijklmnopqrstuvwxyz0123 and minted-secret',
      'minted-secret'
    )
  ).toBe('by [email] with [token] and [token]');
  const long = redactGhOutput(`start${'x'.repeat(50_000)}end`);
  expect(long.startsWith('start')).toBe(true);
  expect(long.endsWith('end')).toBe(true);
  expect(long).toContain('characters omitted');
  expect(long.length).toBeLessThan(33_000);
});

test('gh gets no standard input, so a command that reads it does not wait', async () => {
  await fakeGh('cat; echo done');
  const result = await runGh(['secret', 'set', 'NAME'], {
    token: 'token-value',
    repository: 'a/b',
    signal: AbortSignal.timeout(5000),
    timeoutMs: 2000
  });
  expect(result).toEqual({ ok: true, output: 'done' });
});
