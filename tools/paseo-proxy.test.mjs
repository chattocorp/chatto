// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Exercise route ownership and lifecycle without a running proxy. */
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, mkdir, writeFile, rm, readdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import test from 'node:test';
import { hostname, serviceEnv } from './paseo-proxy.mjs';

const launcher = resolve('tools/paseo-proxy.mjs');

test('URLs use worktree names and rewrite every peer without branch or port dependence', () => {
  const base = 'https://dev.example.test';
  const a = serviceEnv(
    { PASEO_PORT: '1234', PASEO_BRANCH_NAME: 'a' },
    'authling',
    '/work/calm-otter',
    base,
    ['authling', 'dev-full']
  );
  const b = serviceEnv(
    { PASEO_PORT: '5678', PASEO_BRANCH_NAME: 'b' },
    'authling',
    '/work/calm-otter',
    base,
    ['authling', 'dev-full']
  );
  assert.equal(a.PASEO_URL, 'https://authling--calm-otter.dev.example.test');
  assert.equal(a.PASEO_URL, b.PASEO_URL);
  assert.equal(a.PASEO_SERVICE_DEV_FULL_URL, 'https://dev-full--calm-otter.dev.example.test');
  const long = hostname('docs-website', '/work/' + 'a'.repeat(90), new URL(base));
  assert(long.split('.')[0].length <= 63);
  assert.notEqual(long, hostname('docs-website', '/work/' + 'a'.repeat(89) + 'b', new URL(base)));
});

test('route file lifecycle preserves peers and refuses to replace an existing service', async () => {
  await mkdir('.context', { recursive: true });
  const dir = await mkdtemp(resolve('.context/proxy-test-'));
  const routeDir = resolve(dir, 'routes');
  await mkdir(routeDir);
  const configPath = resolve(dir, 'proxy.json');
  await writeFile(configPath, JSON.stringify({ routeDir, baseUrl: 'https://dev.example.test' }));
  await writeFile(
    resolve(dir, 'paseo.json'),
    JSON.stringify({ scripts: { authling: { type: 'service' }, storybook: { type: 'service' } } })
  );
  const children = [];
  function start(name, code, port = '23456', overrides = {}) {
    const child = spawn(process.execPath, [launcher, name, process.execPath, '-e', code], {
      cwd: dir,
      env: {
        ...process.env,
        CHATTO_PASEO_PROXY_CONFIG: configPath,
        PASEO_PORT: port,
        ...overrides
      },
      stdio: ['ignore', 'pipe', 'pipe']
    });
    let output = '';
    child.stdout.on('data', (s) => {
      output += s;
    });
    child.stderr.on('data', (s) => {
      output += s;
    });
    const done = once(child, 'exit');
    children.push({ child, done });
    return { child, done, output: () => output };
  }
  async function ready(child) {
    for (let i = 0; i < 200; i++) {
      if (child.output().includes('READY')) return;
      if (child.child.exitCode !== null) throw new Error(child.output());
      await delay(25);
    }
    throw new Error('Child did not become ready');
  }
  try {
    const code = 'console.log("READY", process.env.PASEO_URL); setInterval(() => {}, 1000);';
    const a = start('authling', code);
    const b = start('storybook', code, '23457');
    await Promise.all([ready(a), ready(b)]);
    assert.equal((await readdir(routeDir)).length, 2);
    const duplicate = start('authling', 'process.exit(0)');
    assert.equal((await duplicate.done)[0], 1);
    assert.equal((await readdir(routeDir)).length, 2);
    a.child.kill('SIGTERM');
    await a.done;
    assert.equal((await readdir(routeDir)).length, 1);
    assert.equal(b.child.exitCode, null);
    const failing = start('authling', 'process.exit(7)');
    assert.equal((await failing.done)[0], 7);
    assert.equal((await readdir(routeDir)).length, 1);
    b.child.kill('SIGTERM');
    await b.done;
    assert.deepEqual(await readdir(routeDir), []);
    const missing = start('authling', 'process.exit(0)', '23456', {
      CHATTO_PASEO_PROXY_CONFIG: resolve(dir, 'missing.json')
    });
    assert.equal((await missing.done)[0], 1);
    const fallback = start('authling', 'console.log(process.env.PASEO_URL)', '23456', {
      CHATTO_PASEO_PROXY_CONFIG: '',
      XDG_CONFIG_HOME: dir,
      PASEO_URL: 'http://original.localhost:6767'
    });
    assert.equal((await fallback.done)[0], 0);
    assert(fallback.output().includes('http://original.localhost:6767'));
  } finally {
    for (const { child } of children)
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
    await Promise.all(children.map(({ done }) => done));
    await rm(dir, { recursive: true, force: true });
    await rm(routeDir, { recursive: true, force: true });
  }
});

test(
  'a terminal process-group interrupt reaches the child only once',
  { timeout: 10000 },
  async () => {
    await mkdir('.context', { recursive: true });
    const dir = await mkdtemp(resolve('.context/proxy-signal-test-'));
    const child = spawn(
      process.execPath,
      [
        launcher,
        'dev',
        process.execPath,
        '-e',
        `
    let signals = 0;
    process.on('SIGINT', () => {
      signals++;
      setTimeout(() => process.exit(signals === 1 ? 0 : 42), 200);
    });
    console.log('READY');
    setInterval(() => {}, 1000);
  `
      ],
      {
        detached: true,
        env: { ...process.env, CHATTO_PASEO_PROXY_CONFIG: '', XDG_CONFIG_HOME: dir },
        stdio: ['ignore', 'pipe', 'pipe']
      }
    );
    const done = once(child, 'exit');
    try {
      await once(child.stdout, 'data');
      // A terminal sends Ctrl-C to its whole foreground process group.
      process.kill(-child.pid, 'SIGINT');
      assert.equal((await done)[0], 0);
    } finally {
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
      await done;
      await rm(dir, { recursive: true, force: true });
    }
  }
);
