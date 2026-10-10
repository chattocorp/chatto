// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Test-only Paseo boundary: allocate fixed fixture ports, expose health, and
 * stop launchers with Paseo's SIGHUP/two-second grace period. Real mise tasks
 * and production launchers own the processes and route files under test.
 */
import { spawn } from 'node:child_process';
import { access, readFile, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { createConnection } from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import { hostname } from '../paseo-proxy.mjs';

const config = JSON.parse(await readFile('fixture.json', 'utf8'));
const [command, action, name] = process.argv.slice(2);
const route = (name) =>
  `${config.routeDir}/${hostname(name, process.cwd(), new URL(config.baseUrl))}.yaml`;
async function exists(path) {
  try {
    await access(path);
    return true;
  } catch {
    return false;
  }
}
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
if (command === 'serve') {
  const server = createServer((_req, res) => res.end('fixture'));
  for (const signal of ['SIGHUP', 'SIGTERM', 'SIGINT'])
    process.on(signal, () => {
      server.closeAllConnections();
      server.close(() => process.exit(0));
    });
  server.listen(config.ports[action], '127.0.0.1');
  await writeFile(`${action}.listener`, String(process.pid));
} else if (command === 'script' && action === 'ls') {
  const status = await Promise.all(
    Object.keys(config.ports).map(async (scriptName) => ({
      scriptName,
      lifecycle: (await exists(route(scriptName))) ? 'running' : 'stopped',
      // Startup must not depend on Paseo reporting healthy services.
      health: 'unhealthy'
    }))
  );
  console.log(JSON.stringify(status));
} else if (command === 'script' && action === 'start') {
  const child = spawn(
    process.execPath,
    ['tools/paseo-proxy.mjs', name, 'mise', 'run', `paseo-${name}`],
    {
      detached: true,
      stdio: 'ignore',
      env: {
        ...process.env,
        PASEO_PORT: String(config.ports[name]),
        PASEO_URL: `https://${name}.example.test`
      }
    }
  );
  await writeFile(`${name}.owner`, String(child.pid));
  child.unref();
  for (let i = 0; i < 100 && !(await exists(route(name))); i++) await delay(10);
  if (!(await exists(route(name)))) throw new Error('Fixture service did not register');
} else if (command === 'script' && action === 'stop') {
  process.kill(Number(await readFile(`${name}.owner`, 'utf8')), 'SIGHUP');
  for (let i = 0; i < 200; i++) {
    if (!(await exists(route(name))) && !(await listening(config.ports[name]))) process.exit(0);
    await delay(10);
  }
  throw new Error('Fixture service exceeded its stop deadline');
} else {
  throw new Error('Unsupported fixture command');
}
