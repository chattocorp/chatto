// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Check the networking boundary without starting Paseo or a development stack. */
import assert from 'node:assert/strict';
import test from 'node:test';
import { execFileSync } from 'node:child_process';
import { mkdir, mkdtemp, writeFile, rm, readFile, access } from 'node:fs/promises';
import { resolve } from 'node:path';
import { mediaAddress, serviceEnvironment, shellQuote } from './paseo-stack.mjs';

test('cleanup skips stopped services and tolerates an exit racing with stop', async (t) => {
  await mkdir('.context', { recursive: true });
  const cwd = await mkdtemp(resolve('.context/paseo-cleanup-test-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  await mkdir(`${cwd}/bin`);
  await mkdir(`${cwd}/.context/paseo-stack/53000`, { recursive: true });
  // The failed stop models a service exiting after the list request. A repeat
  // list must confirm that it stopped before the launcher removes configuration.
  await writeFile(
    `${cwd}/bin/paseo`,
    `#!/usr/bin/env bash
set -eu
if [[ "$2" == ls ]]; then
  status=running
  [[ ! -e stopped ]] || status=stopped
  printf '[{"scriptName":"mailpit","lifecycle":"stopped"},{"scriptName":"livekit","lifecycle":"%s"}]' "$status"
else
  printf '%s' "$3" > stopped
  exit 1
fi
`,
    { mode: 0o700 }
  );
  execFileSync(process.execPath, [resolve('tools/paseo-stack.mjs'), 'cleanup'], {
    cwd,
    env: {
      ...process.env,
      PATH: `${cwd}/bin:${process.env.PATH}`,
      PASEO_PORT: '53000',
      PASEO_URL: 'https://dev.example.test'
    },
    timeout: 10_000
  });
  assert.equal(await readFile(`${cwd}/stopped`, 'utf8'), 'livekit');
  await assert.rejects(access(`${cwd}/.context/paseo-stack/53000`), { code: 'ENOENT' });
});

test('reject missing or unsafe Paseo service input before starting anything', () => {
  for (const port of [undefined, '', '0', '-1', '123.5', '65536']) {
    assert.throws(() =>
      serviceEnvironment({ PASEO_PORT: port, PASEO_URL: 'https://dev.example.test' })
    );
  }
  for (const url of [undefined, 'file:///tmp/test', 'https://user:password@example.test']) {
    assert.throws(() => serviceEnvironment({ PASEO_PORT: '53000', PASEO_URL: url }));
  }
  assert.deepEqual(
    serviceEnvironment({ PASEO_PORT: '53000', PASEO_URL: 'https://dev.example.test/' }),
    {
      port: 53000,
      url: 'https://dev.example.test'
    }
  );
});

test('media must use a local IPv4 interface', async () => {
  assert.equal(await mediaAddress('http://127.0.0.1:6767'), '127.0.0.1');
  assert.equal(await mediaAddress('https://split-dns.invalid', '127.0.0.1'), '127.0.0.1');
  await assert.rejects(mediaAddress('https://split-dns.invalid', '192.0.2.1'), /local IPv4/);
  await assert.rejects(mediaAddress('https://split-dns.invalid', '::1'), /local IPv4/);
});

test('generated shell values remain data, including quotes and command syntax', () => {
  const value = "literal ' quote $(exit 99) `exit 98`\nnext line";
  const actual = execFileSync('bash', ['-c', `value=${shellQuote(value)}; printf '%s' "$value"`], {
    encoding: 'utf8'
  });
  assert.equal(actual, value);
});

test('cleanup attempts every service after a failed stop and retains configuration', async (t) => {
  const cwd = await mkdtemp(resolve('.context/paseo-cleanup-test-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  await mkdir(`${cwd}/bin`);
  const state = `${cwd}/.context/paseo-stack/53000`;
  await mkdir(state, { recursive: true });
  await writeFile(
    `${cwd}/bin/paseo`,
    `#!/usr/bin/env bash
if [[ "$2" == ls ]]; then
  printf '%s' '[{"scriptName":"mailpit","lifecycle":"running"},{"scriptName":"livekit","lifecycle":"running"}]'
else
  printf '%s\\n' "$3" >> attempts
  [[ "$3" != mailpit ]]
fi
`,
    { mode: 0o700 }
  );
  assert.throws(
    () =>
      execFileSync(process.execPath, [resolve('tools/paseo-stack.mjs'), 'cleanup'], {
        cwd,
        env: {
          ...process.env,
          PATH: `${cwd}/bin:${process.env.PATH}`,
          PASEO_PORT: '53000',
          PASEO_URL: 'https://dev.example.test'
        },
        stdio: 'pipe'
      }),
    /Could not stop mailpit/
  );
  assert.deepEqual((await readFile(`${cwd}/attempts`, 'utf8')).trim().split('\n').sort(), [
    'livekit',
    'mailpit'
  ]);
  await access(state);
});

test('watch stops the stack when a previously healthy Chatto exits', async (t) => {
  const cwd = await mkdtemp(resolve('.context/paseo-watch-test-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  await mkdir(`${cwd}/bin`);
  await writeFile(
    `${cwd}/bin/paseo`,
    `#!/usr/bin/env bash
health=healthy
[[ ! -e observed ]] || health=unhealthy
touch observed
printf '[{"scriptName":"dev-full","lifecycle":"running","health":"%s"},{"scriptName":"mailpit","lifecycle":"running","health":"healthy"},{"scriptName":"livekit","lifecycle":"running","health":"healthy"}]' "$health"
`,
    { mode: 0o700 }
  );
  assert.throws(
    () =>
      execFileSync(process.execPath, [resolve('tools/paseo-stack.mjs'), 'watch'], {
        cwd,
        env: { ...process.env, PATH: `${cwd}/bin:${process.env.PATH}` },
        stdio: 'pipe',
        timeout: 5000
      }),
    /Chatto stopped responding/
  );
});
