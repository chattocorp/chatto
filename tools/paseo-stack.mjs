// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Configure a Paseo stack. mise owns process
 * supervision and cleanup; this module never launches a background process.
 */
import { execFile, execFileSync } from 'node:child_process';
import { promisify } from 'node:util';
import { randomBytes } from 'node:crypto';
import { lookup } from 'node:dns/promises';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { networkInterfaces } from 'node:os';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const supportServices = ['mailpit', 'livekit'];
const scripts = () =>
  JSON.parse(
    execFileSync('paseo', ['script', 'ls', '--cwd', process.cwd(), '--json'], {
      encoding: 'utf8',
      timeout: 15_000
    })
  );

/** Quote one generated environment value for a POSIX shell, without eval. */
export const shellQuote = (value) => `'${String(value).replaceAll("'", "'\\''")}'`;

/** Require a valid Paseo listener and public URL before creating state. */
export function serviceEnvironment(env) {
  const port = Number(env.PASEO_PORT);
  if (!Number.isInteger(port) || port < 1024 || port > 65535 || !env.PASEO_URL) {
    throw new Error('Start dev-full through Paseo (PASEO_PORT and PASEO_URL are required).');
  }
  const url = new URL(env.PASEO_URL);
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) {
    throw new Error('PASEO_URL must be an HTTP or HTTPS URL without credentials.');
  }
  return { port, url: url.origin };
}

/** Limit media to an address on this machine; support an explicit split-DNS override. */
export async function mediaAddress(url, override) {
  const local = Object.values(networkInterfaces())
    .flat()
    .filter((entry) => entry?.family === 'IPv4');
  const candidates = override
    ? [{ address: override }]
    : await lookup(new URL(url).hostname, { all: true, family: 4 });
  const address = candidates.find((candidate) =>
    local.some((entry) => entry.address === candidate.address)
  )?.address;
  if (!address)
    throw new Error(
      'The service hostname must resolve to a local IPv4 address; use CHATTO_DEV_LIVEKIT_NODE_IP for split DNS.'
    );
  return address;
}

/** Create configuration only after checking that this run can own the services.
 * Each run uses its allocated dev-full port as its directory name, so old
 * cleanup cannot delete the next run's configuration during a quick restart.
 */
async function prepare() {
  const service = serviceEnvironment(process.env);
  if (
    scripts().some(
      (s) => ['dev', ...supportServices].includes(s.scriptName) && s.lifecycle !== 'stopped'
    )
  ) {
    throw new Error('Stop dev, mailpit, and livekit before starting dev-full.');
  }
  const address = await mediaAddress(service.url, process.env.CHATTO_DEV_LIVEKIT_NODE_IP);
  const livekitURL = new URL(process.env.PASEO_SERVICE_LIVEKIT_URL);
  livekitURL.protocol = livekitURL.protocol === 'https:' ? 'wss:' : 'ws:';
  const listener = createServer();
  await new Promise((res, rej) => {
    listener.once('error', rej);
    listener.listen(0, '127.0.0.1', res);
  });
  const smtpPort = listener.address().port;
  await new Promise((res, rej) => listener.close((error) => (error ? rej(error) : res())));
  // The OS chooses SMTP's port. A bind race fails startup; no adjacent ports
  // are assumed to be reserved. TURN uses the same number in the UDP namespace.
  const env = {
    CHATTO_PASEO_STACK: '1',
    CHATTO_DEV_CHATTO_PORT: service.port,
    // Mailpit reads the resolved port directly; nested mise consumes the override.
    CHATTO_DEV_SMTP_PORT: smtpPort,
    CHATTO_DEV_SMTP_PORT_OVERRIDE: smtpPort,
    CHATTO_SMTP_HOST: '127.0.0.1',
    CHATTO_SMTP_ENABLED: 'true',
    CHATTO_LIVEKIT_ENABLED: 'true',
    CHATTO_DEV_LIVEKIT_URL_OVERRIDE: livekitURL.origin,
    CHATTO_DEV_LIVEKIT_NODE_IP: address,
    CHATTO_LIVEKIT_API_KEY: 'workspace',
    CHATTO_LIVEKIT_API_SECRET: randomBytes(32).toString('hex')
  };
  const dir = `.context/paseo-stack/${service.port}`;
  await mkdir(dir, { recursive: true, mode: 0o700 });
  await writeFile(
    `${dir}/env`,
    Object.entries(env)
      .map(([key, value]) => `export ${key}=${shellQuote(value)}\n`)
      .join(''),
    { mode: 0o600, flag: 'wx' }
  );
}

/** Stop only still-running support services; a failed service is already
 * stopped in Paseo. Keep configuration if the daemon cannot complete cleanup.
 */
async function cleanup() {
  const { port } = serviceEnvironment(process.env);
  const current = scripts();
  // Paseo gives the terminal two seconds to stop. Stop independent support
  // services concurrently so one CLI round trip does not delay the other.
  const results = await Promise.allSettled(
    supportServices.map(async (name) => {
      if (current.find((s) => s.scriptName === name)?.lifecycle === 'stopped') return;
      try {
        await promisify(execFile)(
          'paseo',
          ['script', 'stop', name, '--cwd', process.cwd(), '--json'],
          {
            timeout: 15_000
          }
        );
      } catch (error) {
        // A service can exit between listing it and stopping it.
        if (scripts().find((s) => s.scriptName === name)?.lifecycle !== 'stopped') throw error;
      }
    })
  );
  const failures = supportServices.filter((_, i) => results[i].status === 'rejected');
  if (failures.length)
    throw new Error(`Could not stop ${failures.join(', ')}; configuration retained.`);
  await rm(`.context/paseo-stack/${port}`, { recursive: true, force: true });
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const command = process.argv[2];
    if (command === 'prepare') await prepare();
    else if (command === 'cleanup') await cleanup();
    else throw new Error('Expected prepare or cleanup.');
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
