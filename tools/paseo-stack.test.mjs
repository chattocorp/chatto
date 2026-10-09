// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Check the networking boundary without starting Paseo or a development stack. */
import assert from 'node:assert/strict';
import test from 'node:test';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { livekitConfig, mediaAddress, serviceEnvironment } from './paseo-stack.mjs';

/** Run lifecycle rejection paths in an isolated worktree without a daemon. */
async function isolatedRole(t, role, setup = async () => {}) {
  await mkdir('.context', { recursive: true });
  const cwd = await mkdtemp(resolve('.context/paseo-stack-test-'));
  await setup(cwd);
  const child = spawn(process.execPath, [resolve('tools/paseo-stack.mjs'), role], {
    cwd,
    env: { ...process.env, PASEO_PORT: '53000', PASEO_URL: 'https://dev.example.test' },
    stdio: 'ignore'
  });
  const exited = once(child, 'exit');
  t.after(async () => {
    child.kill('SIGKILL');
    await exited;
    await rm(cwd, { recursive: true, force: true });
  });
  return { child, exited, cwd };
}

test('support services reject standalone starts with a persistent diagnostic', async (t) => {
  const { cwd, exited } = await isolatedRole(t, 'mailpit');
  assert.equal((await exited)[0], 1);
  assert.match(
    await readFile(resolve(cwd, '.context/paseo-mailpit-error.log'), 'utf8'),
    /Start dev-full/
  );
});

test('cancelled restart leaves the previous owner and its state intact', async (t) => {
  const previous = { pid: process.pid, token: 'previous-run' };
  const { child, exited, cwd } = await isolatedRole(t, 'stack', async (cwd) => {
    await mkdir(resolve(cwd, '.context/paseo-stack'), { recursive: true });
    await writeFile(resolve(cwd, '.context/paseo-stack/session.json'), JSON.stringify(previous));
  });
  // The real launcher waits for an existing owner before claiming its state.
  await delay(300);
  child.kill('SIGTERM');
  assert.equal((await exited)[0], 0);
  assert.deepEqual(
    JSON.parse(await readFile(resolve(cwd, '.context/paseo-stack/session.json'), 'utf8')),
    previous
  );
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

test('LiveKit separates loopback HTTP from UDP and uses only local TURN/STUN', () => {
  const config = livekitConfig(
    {
      mediaAddress: '192.0.2.1',
      port: 53000,
      apiKey: 'test',
      apiSecret: 'synthetic-test-secret'
    },
    54000,
    55000
  );
  assert.deepEqual(config.bind_addresses, ['127.0.0.1']);
  assert.equal(config.rtc.tcp_port, 0);
  assert.equal(config.rtc.udp_port, 54000);
  assert.equal(config.rtc.use_external_ip, false);
  assert.equal(config.rtc.enable_loopback_candidate, false);
  assert.deepEqual(config.rtc.ips.includes, ['192.0.2.1/32']);
  assert.equal(config.turn.enabled, true);
  assert.equal(config.turn.udp_port, 55000);
  assert.deepEqual(config.turn.bind_addresses, ['192.0.2.1']);
  assert.deepEqual(config.webhook.urls, ['http://127.0.0.1:53000/webhooks/livekit']);
  assert.equal(config.webhook.api_key, 'test');
  assert.equal(config.keys.test, 'synthetic-test-secret');
});
