// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

// Exercise the actual development tasks without starting or stopping a user's
// stack. Only Chatto is needed: seeding does not require the external services.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createWriteStream } from 'node:fs';
import { chmod, mkdir, mkdtemp, rm, stat } from 'node:fs/promises';
import net from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { stripVTControlCharacters } from 'node:util';
import { setTimeout as delay } from 'node:timers/promises';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
// macOS limits Unix socket paths to 104 bytes; its usual temp path is too long.
const data = await mkdtemp(join(process.platform === 'darwin' ? '/tmp' : tmpdir(), 'seed-'));
await chmod(data, 0o755);
await mkdir(join(root, '.context'), { recursive: true });
const logPath = join(root, '.context/dev-seed-smoke.log');
const log = createWriteStream(logPath);
const children = new Set();
let interrupted = false;

function start(args, env) {
  const child = spawn('mise', args, {
    cwd: root,
    env: { ...process.env, ...env },
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe']
  });
  children.add(child);
  child.diagnostics = [];
  child.stderr.on('data', (chunk) => {
    // Report task/shell failures without dumping backend account/session logs.
    for (const line of stripVTControlCharacters(String(chunk)).split('\n')) {
      if (/Bad substitution|Syntax error|mise ERROR|ERROR task failed/.test(line)) {
        child.diagnostics.push(line);
      }
    }
  });
  child.stdout.pipe(log, { end: false });
  child.stderr.pipe(log, { end: false });
  child.done = new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('close', (code, signal) => resolve({ code, signal }));
  });
  return child;
}

function signal(child, name) {
  try {
    process.kill(-child.pid, name);
  } catch (error) {
    if (error.code !== 'ESRCH') throw error;
  }
}

async function stop(child) {
  signal(child, 'SIGTERM');
  const exited = await Promise.race([
    child.done.then(() => true),
    delay(5000, undefined, { ref: false }).then(() => false)
  ]);
  if (!exited) {
    signal(child, 'SIGKILL');
    await child.done;
  }
  children.delete(child);
}

async function listen(port) {
  const server = net.createServer();
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', resolve);
  });
  return server;
}

async function freeBasePort() {
  for (let attempt = 0; attempt < 20; attempt++) {
    const http = await listen(0);
    const base = http.address().port - 1;
    let nats;
    try {
      if (base + 4 > 65535) continue;
      nats = await listen(base + 4);
      return base;
    } catch (error) {
      if (error.code !== 'EADDRINUSE') throw error;
    } finally {
      await new Promise((resolve) => http.close(resolve));
      if (nats) await new Promise((resolve) => nats.close(resolve));
    }
  }
  throw new Error('Could not reserve development smoke-test ports');
}

// Signal handlers stop only process groups started by this test.
for (const name of ['SIGINT', 'SIGTERM']) {
  process.once(name, () => {
    interrupted = true;
    for (const child of children) signal(child, 'SIGTERM');
    process.exitCode = 1;
  });
}

try {
  const base = await freeBasePort();
  const env = {
    CONDUCTOR_PORT: String(base),
    CHATTO_DEV_DATA_ROOT: data,
    CHATTO_DEV_ROUTE_SUFFIX: `seed-smoke-${process.pid}`,
    PORTLESS_PORT: '42444'
  };
  const backend = start(['run', 'dev-stack-backend'], env);
  const deadline = Date.now() + 180_000;
  const socket = join(data, 'operator/operator.sock');
  while (true) {
    assert.ok(!interrupted, 'Development smoke test interrupted');
    assert.equal(
      backend.exitCode,
      null,
      `Development backend exited: ${backend.diagnostics.join('; ')}; see ${logPath}`
    );
    assert.equal(backend.signalCode, null, `Development backend was stopped; see ${logPath}`);
    let ready = false;
    try {
      ready =
        (
          await fetch(`http://127.0.0.1:${base + 1}/readyz`, {
            signal: AbortSignal.timeout(1000)
          })
        ).ok && (await stat(socket)).isSocket();
    } catch {
      /* Startup has not finished. */
    }
    if (ready) break;
    assert.ok(Date.now() < deadline, `Development backend did not become ready; see ${logPath}`);
    await delay(100);
  }
  assert.equal((await stat(data)).mode & 0o777, 0o755);
  assert.equal((await stat(dirname(socket))).mode & 0o777, 0o700);
  assert.equal((await stat(socket)).mode & 0o777, 0o600);

  const seed = start(
    [
      'seed',
      '--',
      '--seed',
      '42',
      '--users',
      '3',
      '--rooms',
      '2',
      '--messages',
      '8',
      '--thread-replies',
      '2',
      '--json'
    ],
    env
  );
  let output = '';
  seed.stdout.on('data', (chunk) => {
    output += chunk;
  });
  const result = await Promise.race([
    seed.done,
    delay(120_000, undefined, { ref: false }).then(() => {
      throw new Error(`Seed task timed out; see ${logPath}`);
    })
  ]);
  assert.equal(result.code, 0, `Seed task failed; see ${logPath}`);
  // Mise may prefix each output line when it runs as a nested task.
  const lines = stripVTControlCharacters(output)
    .split('\n')
    .map((line) => line.replace(/^\[seed\] /, ''))
    .join('\n');
  const begin = lines.indexOf('{');
  const end = lines.lastIndexOf('}');
  assert.ok(begin >= 0 && end >= begin, 'Seed task did not return a JSON manifest');
  const manifest = JSON.parse(lines.slice(begin, end + 1));
  assert.equal(Number(manifest.seed), 42);
  assert.equal(manifest.users.length, 3);
  assert.equal(manifest.rooms.length, 2);
  assert.equal(manifest.messages.length, 8);
  assert.equal(manifest.messages.filter((message) => message.threadRootId).length, 2);
  for (const room of manifest.rooms) assert.ok(room.memberIds.length > 0);

  // Read a generated account through the same running server, not just the
  // seed response. This also proves the manifest identifies durable resources.
  const get = start(
    [
      'exec',
      '--',
      './cli/bin/chatto',
      'operator',
      'user',
      'get',
      manifest.users[0].id,
      '--operator-socket',
      socket,
      '--json'
    ],
    env
  );
  const readResult = await Promise.race([
    get.done,
    delay(30_000, undefined, { ref: false }).then(() => {
      throw new Error(`Seeded user read timed out; see ${logPath}`);
    })
  ]);
  assert.equal(readResult.code, 0, `Could not read seeded user; see ${logPath}`);
  console.log('Development startup, private socket, seed task, and seeded user read passed.');
} finally {
  for (const child of children) await stop(child);
  log.end();
  await rm(data, { recursive: true, force: true });
}
