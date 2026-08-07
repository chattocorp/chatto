import { spawn, type ChildProcess } from 'node:child_process';
import {
  createWriteStream,
  existsSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
  type WriteStream
} from 'node:fs';
import { createServer, type Server } from 'node:http';
import { finished } from 'node:stream/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import type { TestInfo } from '@playwright/test';

const fixtureDirectory = path.dirname(fileURLToPath(import.meta.url));
const authlingDirectory = path.resolve(fixtureDirectory, '..', '..');

const portsPerTest = 4;
const suitePortRange = 300;
const minimumPort = 30_000;
const maximumPort = 50_000;
const slotCount = Math.floor((maximumPort - minimumPort) / suitePortRange);
const randomSlot = Math.floor(Math.random() * slotCount);
const basePort = process.env.AUTHLING_E2E_BASE_PORT
  ? Number.parseInt(process.env.AUTHLING_E2E_BASE_PORT, 10)
  : minimumPort + randomSlot * suitePortRange;

interface ManagedProcess {
  child: ChildProcess;
  log: WriteStream;
  logPath: string;
  name: string;
  spawnError?: Error;
}

export interface TestStack {
  baseURL: string;
  mailpitURL: string;
  callbackURL: string;
  configPath: string;
  stateDirectory: string;
  authling: ManagedProcess;
  authlingProcesses: ManagedProcess[];
  mailpit: ManagedProcess;
  callbackServer: Server;
  ports: TestPorts;
}

interface TestPorts {
  http: number;
  smtp: number;
  mailpit: number;
  callback: number;
}

function portsForTest(testInfo: TestInfo): TestPorts {
  const offset = (testInfo.workerIndex * 10 + testInfo.parallelIndex) * portsPerTest;
  return {
    http: basePort + offset,
    smtp: basePort + offset + 1,
    mailpit: basePort + offset + 2,
    callback: basePort + offset + 3
  };
}

function startAuthling(
  ports: TestPorts,
  stateDirectory: string,
  configPath: string,
  logPath: string
): ManagedProcess {
  return startProcess(
    'Authling',
    process.env.AUTHLING_E2E_BINARY ?? path.join(authlingDirectory, 'bin', 'authling'),
    ['run', '--config', configPath],
    {
      cwd: authlingDirectory,
      env: {
        ...process.env,
        AUTHLING_HTTP_BIND_ADDRESS: `127.0.0.1:${ports.http}`,
        AUTHLING_HTTP_PUBLIC_URL: `http://127.0.0.1:${ports.http}`,
        AUTHLING_NATS_EMBEDDED_ENABLED: 'true',
        AUTHLING_NATS_EMBEDDED_DATA_DIR: path.join(stateDirectory, 'nats'),
        AUTHLING_SMTP_ENABLED: 'true',
        AUTHLING_SMTP_HOST: '127.0.0.1',
        AUTHLING_SMTP_PORT: String(ports.smtp),
        AUTHLING_SMTP_TLS: 'opportunistic',
        AUTHLING_SMTP_FROM: 'authling-e2e@authling.localhost'
      },
      logPath
    }
  );
}

async function startCallbackServer(port: number): Promise<Server> {
  const server = createServer((_request, response) => {
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    response.end('<!doctype html><title>OIDC callback</title>');
  });
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      server.removeListener('error', reject);
      resolve();
    });
  });
  return server;
}

async function stopCallbackServer(server: Server | undefined): Promise<void> {
  if (!server?.listening) return;
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve()))
  );
}

function startProcess(
  name: string,
  command: string,
  args: string[],
  options: { cwd: string; env: NodeJS.ProcessEnv; logPath: string }
): ManagedProcess {
  const log = createWriteStream(options.logPath, { flags: 'w' });
  const child = spawn(command, args, {
    cwd: options.cwd,
    env: options.env,
    stdio: ['ignore', 'pipe', 'pipe']
  });
  const managed: ManagedProcess = { child, log, logPath: options.logPath, name };
  child.stdout?.pipe(log, { end: false });
  child.stderr?.pipe(log, { end: false });
  child.once('error', (error) => {
    managed.spawnError = error;
  });
  return managed;
}

async function waitForReady(
  process: ManagedProcess,
  url: string,
  timeoutMs = 45_000
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (process.spawnError) {
      throw new Error(`${process.name} failed to start: ${process.spawnError.message}`);
    }
    if (process.child.exitCode !== null || process.child.signalCode !== null) {
      throw new Error(`${process.name} exited before becoming ready; see ${process.logPath}`);
    }
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch {
      // The listener is not ready yet.
    }
    await delay(25);
  }
  throw new Error(`${process.name} did not become ready within ${timeoutMs}ms; see ${process.logPath}`);
}

export async function startStack(testInfo: TestInfo): Promise<TestStack> {
  if (!Number.isInteger(basePort) || basePort < 1 || basePort + suitePortRange > 65_535) {
    throw new Error('AUTHLING_E2E_BASE_PORT must reserve 300 valid TCP ports');
  }
  const ports = portsForTest(testInfo);
  const stateDirectory = mkdtempSync(path.join(os.tmpdir(), 'authling-e2e-'));
  const mailpitURL = `http://127.0.0.1:${ports.mailpit}`;
  const baseURL = `http://127.0.0.1:${ports.http}`;
  const callbackURL = `http://127.0.0.1:${ports.callback}/callback`;
  const configPath = path.join(stateDirectory, 'authling-e2e.toml');
  writeFileSync(
    configPath,
    `[[oidc.clients]]\nid = 'authling-e2e'\nname = 'Authling E2E client'\nredirect_uris = ['${callbackURL}']\n`
  );
  let callbackServer: Server | undefined;
  const mailpit = startProcess(
    'Mailpit',
    process.env.AUTHLING_E2E_MAILPIT_PATH ?? 'mailpit',
    [
      '--database',
      path.join(stateDirectory, 'mailpit.db'),
      '--smtp',
      `127.0.0.1:${ports.smtp}`,
      '--listen',
      `127.0.0.1:${ports.mailpit}`,
      '--disable-version-check',
      '--quiet'
    ],
    {
      cwd: authlingDirectory,
      env: process.env,
      logPath: testInfo.outputPath('mailpit.log')
    }
  );

  let authling: ManagedProcess | undefined;
  try {
    callbackServer = await startCallbackServer(ports.callback);
    await waitForReady(mailpit, `${mailpitURL}/readyz`);
    authling = startAuthling(ports, stateDirectory, configPath, testInfo.outputPath('authling.log'));
    await waitForReady(authling, baseURL);
    return {
      baseURL,
      mailpitURL,
      callbackURL,
      configPath,
      stateDirectory,
      authling,
      authlingProcesses: [authling],
      mailpit,
      callbackServer,
      ports
    };
  } catch (error) {
    await stopManagedProcess(authling);
    await stopManagedProcess(mailpit);
    await stopCallbackServer(callbackServer);
    removeStateDirectory(stateDirectory);
    throw error;
  }
}

export async function stopStack(stack: TestStack, testInfo: TestInfo): Promise<void> {
  await stopManagedProcess(stack.authling);
  await stopManagedProcess(stack.mailpit);
  await stopCallbackServer(stack.callbackServer);
  if (testInfo.status !== testInfo.expectedStatus) {
    for (const [index, process] of stack.authlingProcesses.entries()) {
      await attachLog(testInfo, `authling log ${index + 1}`, process.logPath);
    }
    await attachLog(testInfo, 'mailpit log', stack.mailpit.logPath);
  }
  if (process.env.AUTHLING_E2E_KEEP_STATE === '1') {
    console.error(`Authling E2E state preserved at ${stack.stateDirectory}`);
  } else {
    removeStateDirectory(stack.stateDirectory);
  }
}

export async function restartAuthling(stack: TestStack, testInfo: TestInfo): Promise<void> {
  await stopManagedProcess(stack.authling);
  const restarted = startAuthling(
    stack.ports,
    stack.stateDirectory,
    stack.configPath,
    testInfo.outputPath(`authling-restart-${stack.authlingProcesses.length}.log`)
  );
  stack.authling = restarted;
  stack.authlingProcesses.push(restarted);
  try {
    await waitForReady(restarted, stack.baseURL);
  } catch (error) {
    await stopManagedProcess(restarted);
    throw error;
  }
}

async function stopManagedProcess(process: ManagedProcess | undefined): Promise<void> {
  if (!process) return;
  const child = process.child;
  if (child.exitCode === null && child.signalCode === null) {
    const closed = new Promise<void>((resolve) => child.once('close', () => resolve()));
    child.kill('SIGTERM');
    await Promise.race([closed, delay(5_000)]);
    if (child.exitCode === null && child.signalCode === null) {
      child.kill('SIGKILL');
      await closed;
    }
  }
  process.log.end();
  await finished(process.log);
}

async function attachLog(testInfo: TestInfo, name: string, logPath: string): Promise<void> {
  if (existsSync(logPath)) {
    await testInfo.attach(name, { path: logPath, contentType: 'text/plain' });
  }
}

function removeStateDirectory(directory: string): void {
  if (
    path.dirname(directory) !== os.tmpdir() ||
    !path.basename(directory).startsWith('authling-e2e-')
  ) {
    throw new Error(`refusing to remove unowned E2E state directory: ${directory}`);
  }
  rmSync(directory, { recursive: true });
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
