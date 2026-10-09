// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Optional stable Caddy routes for Paseo services. The child (usually mise)
 * owns its process tree; this launcher forwards stop signals and owns one route.
 */
import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { readFile, realpath } from 'node:fs/promises';
import { homedir } from 'node:os';
import { basename, join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

/** Keep service names in one DNS label, including long worktree names. */
export function hostname(service, workspace, base) {
  let label = `${service}--${basename(workspace)}`
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, '-')
    .replace(/^-+|-+$/g, '');
  if (label.length > 63) {
    label = `${label.slice(0, 52)}--${createHash('sha256').update(label).digest('hex').slice(0, 8)}`;
  }
  return `${label}.${base.hostname}`;
}

/** Replace all peer URLs together so LiveKit and other callers use the same names. */
export function serviceEnv(env, service, workspace, baseUrl, names) {
  const base = new URL(baseUrl);
  if (
    base.protocol !== 'https:' ||
    base.username ||
    base.password ||
    base.pathname !== '/' ||
    base.search ||
    base.hash
  )
    throw new Error('The proxy baseUrl must be an HTTPS origin without credentials.');
  const url = (name) =>
    `${base.protocol}//${hostname(name, workspace, base)}${base.port ? `:${base.port}` : ''}`;
  return {
    ...env,
    ...Object.fromEntries(
      names.map((name) => [
        `PASEO_SERVICE_${name.toUpperCase().replace(/[^A-Z0-9]+/g, '_')}_URL`,
        url(name)
      ])
    ),
    PASEO_URL: url(service)
  };
}

/** Edit only the configured route array. Caddy's ETag protects simultaneous
 * starts/stops and operator edits; retries always read the latest array.
 */
export async function editRoutes(config, change) {
  const admin = new URL(config.admin);
  if (
    admin.protocol !== 'http:' ||
    !['127.0.0.1', '[::1]', 'localhost'].includes(admin.hostname) ||
    admin.username ||
    admin.password
  )
    throw new Error('Caddy admin must be a loopback HTTP endpoint.');
  if (
    !/^\/config\/apps\/http\/servers\/[^/]+\/routes(?:\/\d+\/handle\/\d+\/routes)*$/.test(
      config.routesPath
    )
  )
    throw new Error('routesPath must select a Caddy HTTP route array.');
  const url = new URL(config.routesPath, admin);
  for (let attempt = 0; attempt < 10; attempt++) {
    const response = await fetch(url, {
      headers: { Origin: admin.origin },
      signal: AbortSignal.timeout(5000)
    });
    if (!response.ok) throw new Error(`Cannot read Caddy routes: HTTP ${response.status}`);
    const routes = await response.json();
    const etag = response.headers.get('etag');
    if (!Array.isArray(routes) || !etag)
      throw new Error('Caddy must return a route array and ETag.');
    const next = change(routes);
    const result = await fetch(url, {
      method: 'PATCH',
      headers: { Origin: admin.origin, 'Content-Type': 'application/json', 'If-Match': etag },
      body: JSON.stringify(next),
      signal: AbortSignal.timeout(5000)
    });
    if (result.status === 412) continue;
    if (!result.ok) throw new Error(`Cannot update Caddy routes: HTTP ${result.status}`);
    return;
  }
  throw new Error('Caddy routes changed too often; retry the service start.');
}

/** Wait for the child to exit before removing its route. mise handles signals
 * for its descendants. No separate process groups or background watchdogs.
 */
export async function runService(service, command, args) {
  const configPath =
    process.env.CHATTO_PASEO_PROXY_CONFIG ||
    join(process.env.XDG_CONFIG_HOME || join(homedir(), '.config'), 'chatto/paseo-proxy.json');
  let config;
  try {
    config = JSON.parse(await readFile(configPath, 'utf8'));
  } catch (error) {
    if (error.code !== 'ENOENT' || process.env.CHATTO_PASEO_PROXY_CONFIG) throw error;
  }
  let child;
  let stopped;
  const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
  const handlers = signals.map((signal) => {
    const handler = () => {
      stopped = signal;
      child?.kill(signal);
    };
    process.on(signal, handler);
    return handler;
  });
  const id = `chatto-workspace-${randomUUID()}`;
  let registered = false;
  try {
    let env = process.env;
    if (config) {
      const workspace = await realpath(process.cwd());
      const { scripts } = JSON.parse(await readFile('paseo.json', 'utf8'));
      if (scripts[service]?.type !== 'service') throw new Error('Unknown Paseo service.');
      const port = Number(env.PASEO_PORT);
      if (!Number.isInteger(port) || port < 1024 || port > 65535)
        throw new Error('Start this service through Paseo.');
      env = serviceEnv(
        env,
        service,
        workspace,
        config.baseUrl,
        Object.keys(scripts).filter((name) => scripts[name].type === 'service')
      );
      const host = new URL(env.PASEO_URL).hostname;
      registered = true; // Also clean up if the API accepted a write but its response was lost.
      await editRoutes(config, (routes) => {
        if (routes.some((route) => route.match?.some((match) => match.host?.includes(host))))
          throw new Error(`A route already exists for ${host}; stop its owner before retrying.`);
        return [
          {
            '@id': id,
            match: [{ host: [host] }],
            handle: [
              {
                handler: 'reverse_proxy',
                // Route updates must not immediately close other services' WebSockets.
                stream_close_delay: '5m',
                upstreams: [{ dial: `127.0.0.1:${port}` }]
              }
            ],
            terminal: true
          },
          ...routes
        ];
      });
      console.log(`Workspace URL: ${env.PASEO_URL}`);
    }
    if (stopped) return 1;
    child = spawn(command, args, { env, stdio: 'inherit' });
    return await new Promise((res, rej) => {
      child.once('error', rej);
      child.once('exit', (code) => res(code ?? 1));
    });
  } finally {
    try {
      if (registered)
        await editRoutes(config, (routes) => routes.filter((route) => route['@id'] !== id));
    } finally {
      signals.forEach((signal, i) => process.off(signal, handlers[i]));
    }
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const [service, command, ...args] = process.argv.slice(2);
    if (!service || !command) throw new Error('Expected service name and command.');
    process.exitCode = await runService(service, command, args);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
