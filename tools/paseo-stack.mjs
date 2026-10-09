// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Owns the optional Paseo Chatto/Mailpit/LiveKit stack. Paseo owns each public
 * route; the stack starts its support services, reads their actual ports after
 * startup, and stops only services from this run. Support services require a
 * live owner and stop if that owner disappears, including after SIGKILL.
 * Runtime files and per-run LiveKit credentials stay in the worktree's .context.
 */
import { spawn } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { lookup } from 'node:dns/promises';
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { createServer, isIPv4 } from 'node:net';
import { networkInterfaces } from 'node:os';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';

const stateDir = resolve('.context/paseo-stack');
const supportServices = ['mailpit', 'livekit'];
// A fresh worktree may first need to download and compile the pinned LiveKit tool.
const startupTimeout = 10 * 60_000;

/** Return false only when the process no longer exists. */
function alive(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if (error.code === 'ESRCH') return false;
    throw error;
  }
}

async function readState(name) {
  try {
    return JSON.parse(await readFile(resolve(stateDir, `${name}.json`), 'utf8'));
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  }
}

/** Publish complete state with a private file and atomic rename. */
async function writeState(name, value) {
  const target = resolve(stateDir, `${name}.json`);
  await writeFile(`${target}.${process.pid}`, JSON.stringify(value), { mode: 0o600 });
  await rename(`${target}.${process.pid}`, target);
}

/** Validate Paseo input before starting any process or writing runtime state. */
export function serviceEnvironment(env) {
  const port = Number(env.PASEO_PORT);
  if (!Number.isInteger(port) || port < 1024 || port > 65535 || !env.PASEO_URL) {
    throw new Error('Start this service through Paseo (PASEO_PORT and PASEO_URL are required).');
  }
  const url = new URL(env.PASEO_URL);
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) {
    throw new Error('PASEO_URL must be an HTTP or HTTPS URL without credentials.');
  }
  return { port, url: url.origin };
}

/** Resolve a reachable local IPv4 address, with an explicit override for split DNS. */
export async function mediaAddress(url, override) {
  const addresses = Object.values(networkInterfaces()).flat().filter(Boolean);
  const candidates = override
    ? [{ address: override }]
    : await lookup(new URL(url).hostname, { all: true, family: 4 });
  const address = candidates.find(
    (candidate) =>
      isIPv4(candidate.address) && addresses.some((local) => local.address === candidate.address)
  )?.address;
  if (!address) {
    throw new Error(
      'The service hostname must resolve to a local IPv4 address. Set CHATTO_DEV_LIVEKIT_NODE_IP to a reachable local address for split DNS.'
    );
  }
  return address;
}

/**
 * UDP and TCP have separate port spaces. Media uses the LiveKit service number;
 * built-in TURN/STUN uses the Mailpit service number on the selected interface.
 * Local TURN prevents LiveKit from supplying Google's default STUN servers to
 * browsers. Relays allocate free UDP ports from the OS-backed range below.
 * TCP media is disabled because LiveKit binds that listener on all interfaces.
 */
export function livekitConfig(session, port, turnPort) {
  return {
    port,
    bind_addresses: ['127.0.0.1'],
    rtc: {
      node_ip: session.mediaAddress,
      tcp_port: 0,
      udp_port: port,
      use_external_ip: false,
      enable_loopback_candidate: session.mediaAddress.startsWith('127.'),
      ips: { includes: [`${session.mediaAddress}/32`] }
    },
    turn: {
      enabled: true,
      udp_port: turnPort,
      bind_addresses: [session.mediaAddress],
      relay_range_start: 49152,
      relay_range_end: 65535,
      allow_restricted_peer_cidrs: [`${session.mediaAddress}/32`]
    },
    keys: { [session.apiKey]: session.apiSecret },
    webhook: {
      urls: [`http://127.0.0.1:${session.port}/webhooks/livekit`],
      api_key: session.apiKey
    },
    logging: { level: 'error' }
  };
}

/** Track one process group so signals and startup failures cannot orphan it. */
function run(command, args, { env = process.env, capture = false } = {}) {
  const child = spawn(command, args, {
    env,
    detached: true,
    stdio: capture ? ['ignore', 'pipe', 'pipe'] : 'inherit'
  });
  let stdout = '';
  if (capture) {
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    // CLI output can contain terminal metadata. Report only the failed command.
    child.stderr.resume();
  }
  const done = new Promise((resolveDone) => {
    child.once('error', () => resolveDone({ code: 1, stdout }));
    child.once('close', (code) => resolveDone({ code: code ?? 1, stdout }));
  });
  const signal = (name) => {
    if (!child.pid) return;
    try {
      process.kill(-child.pid, name);
    } catch (error) {
      if (error.code !== 'ESRCH') throw error;
    }
  };
  return {
    done,
    async stop() {
      signal('SIGTERM');
      const exited = await Promise.race([
        done.then(() => true),
        delay(10_000, undefined, { ref: false }).then(() => false)
      ]);
      if (!exited) {
        signal('SIGKILL');
        await done;
      }
    }
  };
}

async function paseo(...args) {
  const { code, stdout } = await run(
    'paseo',
    ['script', ...args, '--cwd', process.cwd(), '--json'],
    { capture: true }
  ).done;
  if (code !== 0) {
    if (stdout.includes('missing from workspace service port plan')) {
      throw new Error(
        'Paseo cached the old service list. Restart the Paseo daemon once, then start dev-full again.'
      );
    }
    throw new Error(`paseo script ${args.join(' ')} failed.`);
  }
  return JSON.parse(stdout);
}

/** Ask the OS for a free loopback SMTP port; Mailpit still detects bind races. */
async function smtpPort() {
  const server = createServer();
  await new Promise((res, rej) => {
    server.once('error', rej);
    server.listen(0, '127.0.0.1', res);
  });
  const port = server.address().port;
  await new Promise((res, rej) => server.close((error) => (error ? rej(error) : res())));
  return port;
}

async function healthy(port, path = '/') {
  try {
    return (await fetch(`http://127.0.0.1:${port}${path}`, { signal: AbortSignal.timeout(1000) }))
      .ok;
  } catch {
    return false;
  }
}

/** All roles react to terminal close as well as explicit stop. */
function cancellation() {
  let stopped = false;
  const stop = () => {
    stopped = true;
  };
  for (const name of ['SIGTERM', 'SIGINT', 'SIGHUP']) process.on(name, stop);
  return () => stopped;
}

async function support(role, service, stopping) {
  const session = await readState('session');
  if (!session || !alive(session.pid))
    throw new Error('Start dev-full; it owns the Mailpit and LiveKit services.');
  let child;
  let monitor;
  try {
    let state = { ...service, token: session.token, pid: process.pid };
    if (role === 'mailpit') {
      state.smtpPort = await smtpPort();
      child = run('mailpit', [
        '--smtp',
        `127.0.0.1:${state.smtpPort}`,
        '--listen',
        `127.0.0.1:${service.port}`,
        '--quiet'
      ]);
    } else {
      const mailpit = await readState('mailpit');
      if (mailpit?.token !== session.token || !alive(mailpit.pid))
        throw new Error('Mailpit must start before LiveKit.');
      const configPath = resolve(stateDir, 'livekit-config.json');
      await writeFile(
        configPath,
        JSON.stringify(livekitConfig(session, service.port, mailpit.port)),
        { mode: 0o600 }
      );
      child = run('mise', [
        'x',
        process.env.CHATTO_DEV_LIVEKIT_SERVER,
        '--',
        'server',
        '--config',
        configPath
      ]);
    }
    let ended = false;
    child.done.then(() => {
      ended = true;
    });
    // Do not publish readiness until the actual service has bound its port.
    const deadline = Date.now() + startupTimeout;
    while (!ended && !stopping() && alive(session.pid) && !(await healthy(service.port))) {
      if (Date.now() > deadline) throw new Error(`${role} did not become ready.`);
      await delay(200);
    }
    if (ended || stopping() || !alive(session.pid))
      throw new Error(`${role} stopped during startup.`);
    await writeState(role, state);
    console.log(`${role}: ${service.url}`);
    monitor = setInterval(async () => {
      if (
        stopping() ||
        !alive(session.pid) ||
        (await readState('session'))?.token !== session.token
      ) {
        clearInterval(monitor);
        await child.stop();
      }
    }, 500);
    const result = await child.done;
    if (!stopping() && alive(session.pid)) throw new Error(`${role} exited (${result.code}).`);
  } finally {
    clearInterval(monitor);
    await child?.stop();
    if ((await readState(role))?.pid === process.pid)
      await rm(resolve(stateDir, `${role}.json`), { force: true });
  }
}

/** Wait for state from this run; never use a stale port from Paseo's peer env. */
async function ready(role, token, stopping) {
  const deadline = Date.now() + startupTimeout;
  while (!stopping()) {
    const state = await readState(role);
    if (state?.token === token && alive(state.pid)) return state;
    const scripts = await paseo('ls');
    if (scripts.find((script) => script.scriptName === role)?.lifecycle === 'stopped') {
      throw new Error(`${role} failed to start; inspect its Paseo terminal.`);
    }
    if (Date.now() > deadline) throw new Error(`${role} did not become ready.`);
    await delay(500);
  }
  throw new Error('Stack start cancelled.');
}

async function stack(service, stopping) {
  await mkdir(resolve('.context'), { recursive: true });
  const previous = await readState('session');
  // Paseo removes a route before the old process has finished cleanup. A quick
  // stop/start must wait for that cleanup before claiming the same worktree.
  const cleanupDeadline = Date.now() + 20_000;
  while (previous && alive(previous.pid) && Date.now() < cleanupDeadline && !stopping()) {
    await delay(200);
  }
  if (stopping()) return;
  if (previous && alive(previous.pid)) throw new Error('This workspace already has a stack owner.');
  if (previous) {
    for (const role of supportServices) {
      if (alive((await readState(role))?.pid))
        throw new Error('Previous support services are still stopping. Retry shortly.');
    }
    await rm(stateDir, { recursive: true, force: true });
  }
  // mkdir is the workspace lock: concurrent starts cannot replace the owner.
  await mkdir(stateDir, { mode: 0o700 });
  const started = [];
  let child;
  try {
    const scripts = await paseo('ls');
    if (
      scripts.some(
        (script) =>
          ['dev', ...supportServices].includes(script.scriptName) && script.lifecycle !== 'stopped'
      )
    ) {
      throw new Error(
        'Stop dev, mailpit, and livekit before starting dev-full. Existing services were left running.'
      );
    }
    const session = {
      ...service,
      pid: process.pid,
      token: randomUUID(),
      apiKey: 'workspace',
      apiSecret: randomBytes(32).toString('hex'),
      mediaAddress: await mediaAddress(service.url, process.env.CHATTO_DEV_LIVEKIT_NODE_IP)
    };
    await writeState('session', session);
    await writeFile(resolve(stateDir, 'owner'), String(process.pid), { mode: 0o600 });
    const states = {};
    for (const role of supportServices) {
      if (stopping()) return;
      started.push(role);
      await paseo('start', role);
      states[role] = await ready(role, session.token, stopping);
    }
    const livekitURL = new URL(states.livekit.url);
    livekitURL.protocol = livekitURL.protocol === 'https:' ? 'wss:' : 'ws:';
    child = run('bash', ['tools/paseo-dev.sh'], {
      env: {
        ...process.env,
        CHATTO_PASEO_STACK_OWNER: String(process.pid),
        CHATTO_SMTP_ENABLED: 'true',
        CHATTO_SMTP_HOST: '127.0.0.1',
        CHATTO_DEV_SMTP_PORT: String(states.mailpit.smtpPort),
        CHATTO_LIVEKIT_ENABLED: 'true',
        CHATTO_DEV_LIVEKIT_URL: livekitURL.origin,
        CHATTO_LIVEKIT_API_KEY: session.apiKey,
        CHATTO_LIVEKIT_API_SECRET: session.apiSecret
      }
    });
    let result;
    child.done.then((value) => {
      result = value;
    });
    while (!stopping() && !result) {
      for (const role of supportServices) {
        const state = await readState(role);
        if (state?.token !== session.token || !alive(state.pid))
          throw new Error(`${role} stopped; stopping this stack.`);
      }
      await delay(500);
    }
    if (!stopping() && result?.code) throw new Error(`Chatto exited (${result.code}).`);
  } finally {
    await child?.stop();
    for (const role of started.reverse()) {
      await paseo('stop', role).catch(() => {
        console.error(`Could not stop ${role} through Paseo; its owner monitor will stop it.`);
      });
    }
    await rm(stateDir, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const role = process.argv[2];
  try {
    const service = serviceEnvironment(process.env);
    const stopping = cancellation();
    if (role === 'stack') await stack(service, stopping);
    else if (supportServices.includes(role)) await support(role, service, stopping);
    else throw new Error('Expected stack, mailpit, or livekit.');
  } catch (error) {
    console.error(error.message);
    // Paseo closes an exec-based terminal on exit. Keep the final diagnostic
    // available after a failed start without retaining process output or secrets.
    if (['stack', ...supportServices].includes(role)) {
      await mkdir(resolve('.context'), { recursive: true });
      await writeFile(resolve(`.context/paseo-${role}-error.log`), `${error.message}\n`, {
        mode: 0o600
      }).catch(() => {});
    }
    process.exitCode = 1;
  }
}
