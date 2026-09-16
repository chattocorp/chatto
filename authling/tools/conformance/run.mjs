// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later
import { spawn, execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, readFile, writeFile, open, unlink } from 'node:fs/promises';
import { randomBytes, createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import net from 'node:net';

const exec = promisify(execFile);
const source = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(source, '../..');
const state = path.join(root, '.authling/conformance');
const suite = 'https://conformance.localhost:8443';
const issuer = 'https://conformance.localhost:9443';
// Do not let deployment credentials or ordinary development settings enter this instance.
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('AUTHLING_')));
Object.assign(env, { CONFORMANCE_SOURCE: source, CONFORMANCE_STATE: state });
const project = `authling-conformance-${createHash('sha256').update(root).digest('hex').slice(0, 10)}`;
const composeArgs = ['compose', '-p', project, '-f', path.join(source, 'compose.yml')];
const children = [];
let stopping = false;
let composeStarted = false;
let locked = false;

for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
  process.on(signal, () => {
    stopping = true;
    for (const child of children) child.kill('SIGTERM');
  });
}

function checkRunning() {
  if (stopping) throw new Error('Stopped');
  if (children.some((child) => child.exitCode !== null || child.signalCode !== null)) {
    throw new Error('A local test process stopped unexpectedly.');
  }
}

function start(command, args) {
  // Protocol and email logs can contain test credentials. Keep them out of terminal logs.
  const child = spawn(command, args, { cwd: root, env, stdio: 'ignore' });
  child.on('error', () => { stopping = true; process.exitCode = 1; });
  children.push(child);
}

async function curl(url, body) {
  const args = ['--silent', '--show-error', '--fail', '--insecure', '--max-time', '5',
    '--resolve', 'conformance.localhost:8443:127.0.0.1',
    '--resolve', 'conformance.localhost:9443:127.0.0.1', url];
  if (body !== undefined) args.push('-X', 'POST');
  if (body !== undefined && body !== null) args.push('-H', 'Content-Type: application/json', '--data-binary', `@${body}`);
  const { stdout } = await exec('curl', args, { env });
  return stdout;
}

async function api(route, body) {
  const text = await curl(suite + route, body);
  return text ? JSON.parse(text) : {};
}

async function ready(url) {
  for (let attempt = 0; attempt < 90; attempt++) {
    checkRunning();
    try { await curl(url); return; } catch { await delay(1000); }
  }
  throw new Error('Local conformance service did not become ready within the startup limit.');
}

async function availablePort(port) {
  const server = net.createServer();
  await new Promise((resolve, reject) => {
    server.once('error', () => reject(new Error(`Port ${port} is in use. Stop the other local test session first.`)));
    server.listen(port, '127.0.0.1', resolve);
  });
  await new Promise((resolve) => server.close(resolve));
}

async function main() {
  if (process.platform !== 'darwin') throw new Error('This local setup currently requires macOS with Docker Desktop or OrbStack.');
  await exec('docker', ['info'], { env });
  await exec('docker', ['compose', 'version'], { env });
  await mkdir(state, { recursive: true, mode: 0o700 });
  try {
    const lock = await open(path.join(state, 'run.lock'), 'wx', 0o600);
    locked = true;
    await lock.writeFile(String(process.pid));
    await lock.close();
  } catch {
    throw new Error(`A conformance session is already running. If a previous run was killed, stop its containers and remove ${state}/run.lock.`);
  }
  for (const port of [8443, 9443, 19400, 19408, 19409]) await availablePort(port);
  let secret;
  try { secret = (await readFile(path.join(state, 'client-secret'), 'utf8')).trim(); }
  catch (error) {
    if (error.code !== 'ENOENT') throw error;
    secret = randomBytes(32).toString('hex');
    await writeFile(path.join(state, 'client-secret'), secret, { mode: 0o600 });
  }
  // JSON string quoting is also valid TOML basic-string quoting for these paths and values.
  const q = JSON.stringify;
  await writeFile(path.join(state, 'authling.toml'), `[site]
name = 'Authling Conformance'
[http]
bind_address = '127.0.0.1:19400'
public_url = '${issuer}'
trust_proxy_headers = true
[nats.embedded]
enabled = true
data_dir = ${q(path.join(state, 'nats'))}
[smtp]
enabled = true
host = '127.0.0.1'
port = 19408
tls = 'opportunistic'
from = 'test@authling.localhost'
[[oidc.clients]]
id = 'conformance'
name = 'Local conformance suite'
secret = ${q(secret)}
require_pkce = false
redirect_uris = ['${suite}/test/a/authling/callback']
[[oidc.clients]]
id = 'conformance2'
name = 'Second local conformance client'
secret = ${q(secret + '-second')}
require_pkce = false
redirect_uris = ['${suite}/test/a/authling/callback']
`, { mode: 0o600 });
  const configPath = path.join(state, 'suite-config.json');
  await writeFile(configPath, JSON.stringify({ alias: 'authling', description: 'Local Authling conformance',
    server: { discoveryUrl: `${issuer}/.well-known/openid-configuration` },
    client: { client_id: 'conformance', client_secret: secret, scope: 'openid' },
    client2: { client_id: 'conformance2', client_secret: secret + '-second', scope: 'openid' },
    // Exercise the unsupported POST authentication method, rather than fail on missing test configuration.
    client_secret_post: { client_id: 'conformance', client_secret: secret, scope: 'openid' }
  }), { mode: 0o600 });
  checkRunning();
  start('mailpit', ['--smtp', '127.0.0.1:19408', '--listen', '127.0.0.1:19409', '--disable-version-check', '--quiet']);
  start(path.join(root, 'bin/authling'), ['run', '--config', path.join(state, 'authling.toml')]);
  console.log('Starting the pinned OpenID conformance suite (first run downloads Docker images)…');
  composeStarted = true;
  await exec('docker', [...composeArgs, 'up', '-d'], { env, timeout: 300_000 });
  await ready(`${suite}/api/plan?length=1`);
  await ready(`${issuer}/.well-known/openid-configuration`);
  const plan = await api('/api/plan?planName=oidcc-config-certification-test-plan', configPath);
  const test = await api(`/api/runner?test=oidcc-discovery-endpoint-verification&plan=${plan.id}`, null);
  await api(`/api/runner/${test.id}`, null);
  let result;
  for (let attempt = 0; attempt < 60; attempt++) {
    checkRunning();
    result = await api(`/api/info/${test.id}`);
    if (['FINISHED', 'INTERRUPTED'].includes(result.status)) break;
    await delay(1000);
  }
  console.log(`Discovery: ${result.result ?? result.status}`);
  const variant = encodeURIComponent(JSON.stringify({ server_metadata: 'discovery', client_registration: 'static_client' }));
  const basic = await api(`/api/plan?planName=oidcc-basic-certification-test-plan&variant=${variant}`, configPath);
  const pkce = await api(`/api/runner?test=oidcc-ensure-request-with-valid-pkce-succeeds&plan=${basic.id}`, null);
  await api(`/api/runner/${pkce.id}`, null);
  await writeFile(path.join(state, 'last-run.json'), JSON.stringify({ discovery: { plan: plan.id, test: test.id, result: result.result }, basic: basic.id, pkce: pkce.id }, null, 2));
  console.log(`PKCE test (complete login and consent in Chrome):\n${suite}/log-detail.html?log=${pkce.id}`);
  console.log(`Suite: ${suite}/\nAuthling signup: ${issuer}/signup\nMailpit: http://127.0.0.1:19409/`);
  console.log('Accept the local self-signed certificate in the test browser. Use synthetic accounts only.');
  console.log('The confidential test clients explicitly allow requests without PKCE. This is not a full conformance run.');
  console.log('Press Ctrl-C to stop all test services. Test state and suite results are preserved.');
  if (result.result !== 'PASSED') process.exitCode = 1;
  while (!stopping) { checkRunning(); await delay(500); }
}

try { await main(); }
catch (error) {
  if (!stopping) {
    // exec errors can include request bodies or service output; do not print those.
    console.error(error.cmd ? 'A setup or suite HTTP command failed. Check Docker, curl, and the local service ports.' : error.message);
    process.exitCode = 1;
  }
} finally {
  for (const child of children) child.kill('SIGTERM');
  await Promise.all(children.map(async (child) => {
    for (let i = 0; i < 50 && child.exitCode === null && child.signalCode === null; i++) await delay(100);
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
  }));
  if (composeStarted) {
    try { await exec('docker', [...composeArgs, 'down'], { env, timeout: 30_000 }); }
    catch { console.error(`Container cleanup failed. Run docker compose -p ${project} -f tools/conformance/compose.yml down with CONFORMANCE_SOURCE and CONFORMANCE_STATE set.`); process.exitCode = 1; }
  }
  if (locked) await unlink(path.join(state, 'run.lock'));
}
