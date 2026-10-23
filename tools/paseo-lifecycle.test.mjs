// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Run the repository's mise task graph, not a model of its shutdown behavior.
 * External apps are small HTTP listeners; only the Paseo CLI boundary is faked.
 * The fixture never starts or stops a user's real Paseo services.
 */
import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { once } from 'node:events';
import { copyFile, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { createServer, createConnection } from 'node:net';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import test from 'node:test';
import { promisify } from 'node:util';

async function listening(port) {
  return new Promise((resolve) => {
    const socket = createConnection({ host: '127.0.0.1', port });
    socket.once('connect', () => {
      socket.destroy();
      resolve(true);
    });
    socket.once('error', () => resolve(false));
  });
}

test(
  'Paseo stop closes every real mise listener and removes routes and stack state',
  { timeout: 30000 },
  async () => {
    await mkdir('.context', { recursive: true });
    const dir = await mkdtemp(resolve('.context/mise-lifecycle-'));
    const reservations = [];
    const ports = {};
    let child;
    let exited;
    let output = '';
    try {
      for (const name of ['dev-full', 'mailpit', 'livekit']) {
        const server = createServer();
        await new Promise((res) => server.listen(0, '127.0.0.1', res));
        reservations.push(server);
        ports[name] = server.address().port;
      }
      await mkdir(`${dir}/tools/fixtures`, { recursive: true });
      await mkdir(`${dir}/bin`);
      await mkdir(`${dir}/routes`);
      for (const file of [
        'paseo-proxy.mjs',
        'paseo-stack.mjs',
        'paseo-dev.sh',
        'fixtures/paseo-lifecycle.mjs'
      ])
        await copyFile(`tools/${file}`, `${dir}/tools/${file}`);
      const config = { ports, routeDir: `${dir}/routes`, baseUrl: 'https://dev.example.test' };
      await writeFile(`${dir}/fixture.json`, JSON.stringify(config));
      await writeFile(
        `${dir}/paseo.json`,
        JSON.stringify({
          scripts: Object.fromEntries(Object.keys(ports).map((name) => [name, { type: 'service' }]))
        })
      );
      // Copy the checked-in task definitions so a lifecycle regression in mise.toml fails this test.
      const source = await readFile('mise.toml', 'utf8');
      const tasks = ['paseo-stack', 'paseo-prepare', 'paseo-chatto', 'paseo-cleanup'];
      let toml = tasks
        .map((name) => {
          const start = source.indexOf(`[tasks.${name}]\n`);
          assert(start >= 0, `missing real task ${name}`);
          const end = source.indexOf('\n[', start + 1);
          return source.slice(start, end < 0 ? undefined : end);
        })
        .join('\n');
      toml += '\n[tasks.build-dev-cli]\nrun = "true"\n';
      for (const [task, name] of [
        ['dev', 'dev-full'],
        ['paseo-mailpit', 'mailpit'],
        ['paseo-livekit', 'livekit']
      ])
        toml += `\n[tasks.${task}]\nrun = "exec node tools/fixtures/paseo-lifecycle.mjs serve ${name}"\n`;
      await writeFile(`${dir}/mise.toml`, toml);
      await writeFile(
        `${dir}/bin/paseo`,
        '#!/usr/bin/env bash\nexec node tools/fixtures/paseo-lifecycle.mjs "$@"\n',
        { mode: 0o700 }
      );
      const env = {
        ...process.env,
        PATH: `${dir}/bin:${process.env.PATH}`,
        MISE_TRUSTED_CONFIG_PATHS: dir,
        CHATTO_PASEO_PROXY_CONFIG: `${dir}/fixture.json`,
        CHATTO_DEV_LIVEKIT_NODE_IP: '127.0.0.1',
        PASEO_PORT: String(ports['dev-full']),
        PASEO_URL: 'https://branch.example.test',
        PASEO_SERVICE_LIVEKIT_URL: 'https://livekit.example.test'
      };
      await Promise.all(reservations.map((server) => new Promise((res) => server.close(res))));
      child = spawn(
        process.execPath,
        ['tools/paseo-proxy.mjs', 'dev-full', 'mise', 'run', 'paseo-stack'],
        {
          cwd: dir,
          env,
          detached: true,
          stdio: ['ignore', 'pipe', 'pipe']
        }
      );
      exited = once(child, 'exit');
      for (const stream of [child.stdout, child.stderr])
        stream.on('data', (data) => {
          output += data;
        });
      for (let i = 0; i < 150; i++) {
        if ((await Promise.all(Object.values(ports).map(listening))).every(Boolean)) break;
        assert.equal(child.exitCode, null, output);
        await delay(50);
      }
      assert((await Promise.all(Object.values(ports).map(listening))).every(Boolean), output);
      // Once startup is complete, losing a support service must leave Chatto running.
      await promisify(execFile)(`${dir}/bin/paseo`, ['script', 'stop', 'mailpit'], {
        cwd: dir,
        env
      });
      await delay(1500);
      assert.equal(child.exitCode, null, output);
      assert.equal(await listening(ports['dev-full']), true, output);
      assert.equal(await listening(ports.mailpit), false, output);
      child.kill('SIGHUP');
      // Paseo forcibly kills the launcher after two seconds. A deadline miss must fail,
      // even if a detached child later finishes and makes the socket assertion pass.
      const forced = setTimeout(() => child.kill('SIGKILL'), 2000);
      const [, signal] = await exited;
      clearTimeout(forced);
      assert.notEqual(signal, 'SIGKILL', `Shutdown exceeded Paseo's deadline:\n${output}`);
      assert.deepEqual(
        await Promise.all(Object.values(ports).map(listening)),
        [false, false, false],
        output
      );
      assert.deepEqual(await readdir(`${dir}/routes`), []);
      assert.deepEqual(await readdir(`${dir}/.context/paseo-stack`), []);
    } finally {
      if (child && child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
      if (exited) await exited;
      // Failure cleanup targets only fixture processes whose IDs were recorded here.
      for (const file of await readdir(dir)) {
        if (!/\.(owner|listener)$/.test(file)) continue;
        try {
          process.kill(Number(await readFile(`${dir}/${file}`, 'utf8')), 'SIGTERM');
        } catch (error) {
          if (error.code !== 'ESRCH') throw error;
        }
      }
      for (const server of reservations) if (server.listening) server.close();
      await rm(dir, { recursive: true, force: true });
    }
  }
);
