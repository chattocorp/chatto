// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Exercise Traefik's file watcher, stream isolation, and restart recovery. */
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { once } from 'node:events';
import { mkdtemp, mkdir, rm } from 'node:fs/promises';
import { createServer, get } from 'node:http';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import test from 'node:test';
import { registerRoute } from './paseo-proxy.mjs';

/** Wait for a file-provider change, including its configured debounce interval. */
async function eventually(check) {
  let error;
  for (let i = 0; i < 100; i++) {
    try {
      return await check();
    } catch (e) {
      error = e;
    }
    await delay(50);
  }
  throw error;
}

test(
  'route changes preserve streams; proxy restart restores routes; removed hosts return 404',
  { timeout: 30000 },
  async () => {
    await mkdir('.context', { recursive: true });
    const dir = await mkdtemp(resolve('.context/traefik-test-'));
    const sockets = [];
    const upstream = createServer((req, res) => res.end(req.headers.host));
    upstream.on('upgrade', (req, socket) => {
      sockets.push(socket);
      const accept = createHash('sha1')
        .update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11')
        .digest('base64');
      socket.write(
        `HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`
      );
    });
    await new Promise((res) => upstream.listen(0, '127.0.0.1', res));
    const reservation = createServer();
    await new Promise((res) => reservation.listen(0, '127.0.0.1', res));
    const httpPort = reservation.address().port;
    await new Promise((res) => reservation.close(res));
    const url = `http://127.0.0.1:${httpPort}`;
    let proxy;
    let exited;
    let output = '';
    let removeA;
    let removeB;
    // Node fetch can replace Host; use HTTP directly to exercise virtual hosts.
    const response = (host, path = '/') =>
      new Promise((res, rej) => {
        get(url + path, { headers: { Host: host } }, (response) => {
          let body = '';
          response.setEncoding('utf8');
          response.on('data', (chunk) => {
            body += chunk;
          });
          response.on('end', () =>
            res({ status: response.statusCode, body, location: response.headers.location })
          );
          response.on('error', rej);
        }).on('error', rej);
      });
    async function status(host, expected) {
      await eventually(async () => assert.equal((await response(host)).status, expected));
    }
    async function start() {
      proxy = spawn(
        'traefik',
        [
          `--entrypoints.web.address=127.0.0.1:${httpPort}`,
          `--providers.file.directory=${dir}`,
          '--providers.file.watch=true',
          '--providers.providersThrottleDuration=100ms',
          '--global.checknewversion=false',
          '--global.sendanonymoususage=false'
        ],
        { stdio: ['ignore', 'pipe', 'pipe'] }
      );
      for (const stream of [proxy.stdout, proxy.stderr])
        stream.on('data', (s) => {
          output += s;
        });
      exited = once(proxy, 'exit');
      await status('127.0.0.1', 200);
    }
    try {
      removeA = await registerRoute(dir, '127.0.0.1', upstream.address().port, {
        from: 'https://branch.example.test',
        to: 'https://stable.example.test'
      });
      await start();
      await status('unknown.example.test', 404);
      const redirect = await response('branch.example.test', '/login?next=%2Fchat');
      assert.equal(redirect.status, 302);
      assert.equal(redirect.location, 'https://stable.example.test/login?next=%2Fchat');
      const ws = new WebSocket(url.replace('http:', 'ws:'));
      await new Promise((res, rej) => {
        ws.onopen = res;
        ws.onerror = rej;
      });
      for (let i = 0; i < 3; i++) {
        removeB = await registerRoute(dir, 'b.example.test', upstream.address().port);
        await status('b.example.test', 200);
        assert.equal((await response('b.example.test')).body, 'b.example.test');
        await removeB();
        removeB = undefined;
        await status('b.example.test', 404);
        assert.equal(ws.readyState, WebSocket.OPEN);
        const received = new Promise((res) => {
          ws.onmessage = (event) => res(event.data);
        });
        sockets[0].write(Buffer.from([0x81, 2, 111, 107]));
        assert.equal(await received, 'ok');
      }
      // Restart only the proxy. The upstream and route owner remain running.
      proxy.kill('SIGTERM');
      await exited;
      await start();
      await removeA();
      removeA = undefined;
      await status('127.0.0.1', 404);
      await status('branch.example.test', 404);
      removeA = await registerRoute(dir, '127.0.0.1', upstream.address().port);
      await status('127.0.0.1', 200);
    } catch (error) {
      throw new Error(`${error.message}\nIsolated Traefik output:\n${output}`, { cause: error });
    } finally {
      if (proxy && proxy.exitCode === null && proxy.signalCode === null) proxy.kill('SIGTERM');
      if (exited) await exited;
      await removeB?.();
      await removeA?.();
      sockets.forEach((socket) => socket.destroy());
      upstream.closeAllConnections();
      await new Promise((res) => upstream.close(res));
      await rm(dir, { recursive: true, force: true });
    }
  }
);
