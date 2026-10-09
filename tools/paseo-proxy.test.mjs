// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Exercise route ownership and lifecycle against a local Caddy API stand-in. */
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { createServer } from 'node:http';
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

test('concurrent routes, duplicate rejection, stop, and child failure preserve unrelated routes', async () => {
  await mkdir('.context', { recursive: true });
  const dir = await mkdtemp(resolve('.context/proxy-test-'));
  let routes = [{ '@id': 'operator-route', handle: [{ handler: 'static_response' }] }];
  let version = 0;
  let conflicts = 0;
  const server = createServer(async (req, res) => {
    if (req.method === 'GET') {
      res.setHeader('ETag', `"${version}"`);
      res.end(JSON.stringify(routes));
      return;
    }
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    // Force one real optimistic-concurrency retry as well as any natural races.
    if (conflicts === 0 || req.headers['if-match'] !== `"${version}"`) {
      conflicts++;
      res.writeHead(412).end();
      return;
    }
    routes = JSON.parse(Buffer.concat(chunks));
    version++;
    res.end();
  });
  await new Promise((res) => server.listen(0, '127.0.0.1', res));
  const configPath = resolve(dir, 'proxy.json');
  await writeFile(
    configPath,
    JSON.stringify({
      admin: `http://127.0.0.1:${server.address().port}`,
      routesPath: '/config/apps/http/servers/test/routes',
      baseUrl: 'https://dev.example.test'
    })
  );
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
    children.push(child);
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
    assert.equal(routes.length, 3);
    assert(a.output().includes('https://authling--proxy-test-'));
    const duplicate = start('authling', 'process.exit(0)');
    assert.equal((await duplicate.done)[0], 1);
    assert.equal(routes.length, 3);
    a.child.kill('SIGTERM');
    await a.done;
    assert.equal(routes.length, 2);
    assert.equal(b.child.exitCode, null);
    const failing = start('authling', 'process.exit(7)');
    assert.equal((await failing.done)[0], 7);
    assert.equal(routes.length, 2);
    b.child.kill('SIGTERM');
    await b.done;
    assert.deepEqual(
      routes.map((r) => r['@id']),
      ['operator-route']
    );
    assert(conflicts > 0);
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
    for (const child of children)
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
    await new Promise((res) => server.close(res));
    await rm(dir, { recursive: true, force: true });
  }
});
