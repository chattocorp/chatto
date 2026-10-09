// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Optional stable Traefik routes for Paseo services. The child (usually mise)
 * owns its process tree; this launcher forwards stop signals and owns one route file.
 */
import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { link, readFile, realpath, rm, writeFile } from 'node:fs/promises';
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

/** Publish a complete route atomically, without replacing another launch's file.
 * An optional Paseo URL redirects to the stable URL. Traefik watches the
 * directory; removing this file withdraws both routes.
 * After a forced kill, stop the old service before removing its stale route file.
 */
export async function registerRoute(routeDir, host, port, redirect) {
  const file = join(routeDir, `${host}.yaml`);
  const pending = join(routeDir, `${randomUUID()}.pending`);
  const config = {
    http: {
      routers: { [host]: { rule: `Host(\`${host}\`)`, service: host } },
      services: { [host]: { loadBalancer: { servers: [{ url: `http://127.0.0.1:${port}` }] } } }
    }
  };
  if (redirect && new URL(redirect.from).hostname !== host) {
    const alias = `${host}-paseo`;
    config.http.routers[alias] = {
      rule: `Host(\`${new URL(redirect.from).hostname}\`)`,
      service: host,
      middlewares: [alias]
    };
    // Keep paths and queries, but always use the workspace's canonical origin.
    // Temporary redirects avoid caching a branch URL after another workspace uses it.
    config.http.middlewares = {
      [alias]: {
        redirectRegex: {
          regex: '^https?://[^/]+(.*)$',
          replacement: `${new URL(redirect.to).origin}\${1}`,
          permanent: false
        }
      }
    };
  }
  try {
    // JSON is valid YAML. The provider ignores the incomplete .pending file.
    await writeFile(pending, JSON.stringify(config), { flag: 'wx', mode: 0o640 });
    await link(pending, file);
  } finally {
    await rm(pending, { force: true });
  }
  return () => rm(file);
}

/** Wait for the child to exit before removing its route. mise handles signals
 * for its descendants. Isolate the child from terminal signals so forwarding
 * delivers each signal once, including while mise runs its cleanup tasks.
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
  let removeRoute;
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
      if (!config.routeDir?.startsWith('/'))
        throw new Error('Configure an absolute routeDir watched by Traefik.');
      removeRoute = await registerRoute(
        config.routeDir,
        new URL(env.PASEO_URL).hostname,
        port,
        process.env.PASEO_URL ? { from: process.env.PASEO_URL, to: env.PASEO_URL } : undefined
      );
      console.log(`Workspace URL: ${env.PASEO_URL}`);
    }
    if (stopped) return 1;
    child = spawn(command, args, { env, stdio: 'inherit', detached: true });
    return await new Promise((res, rej) => {
      child.once('error', rej);
      child.once('exit', (code) => res(code ?? 1));
    });
  } finally {
    try {
      await removeRoute?.();
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
