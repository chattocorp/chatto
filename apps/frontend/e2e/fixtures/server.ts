import { spawn, type ChildProcess } from 'child_process';
import { chmodSync, createWriteStream, existsSync, mkdirSync, mkdtempSync, rmSync } from 'fs';
import os from 'os';
import path from 'path';
import { fileURLToPath } from 'url';
import type { TestInfo } from '@playwright/test';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export interface ServerInfo {
  baseURL: string;
  port: number;
  process: ChildProcess;
  executablePath: string;
  dataDir: string;
  searchDirectory?: string;
  logPath: string;
  operatorSocketPath?: string;
}

const PORT_STRIDE = 1;

// Random offset for this test suite run to avoid port collisions
// when running multiple test suites simultaneously.
// Each suite needs ~100 ports (10 workers × 10 parallel tests).
// Range 4040-30000 gives ~260 slots, making collisions very unlikely.
const SUITE_PORT_RANGE = 100;
const MIN_PORT = 4040;
const MAX_PORT = 30000;
const SLOT_COUNT = Math.floor((MAX_PORT - MIN_PORT) / SUITE_PORT_RANGE);
const RANDOM_SLOT = Math.floor(Math.random() * SLOT_COUNT);

const BASE_PORT = process.env.E2E_BASE_PORT
  ? parseInt(process.env.E2E_BASE_PORT, 10)
  : MIN_PORT + RANDOM_SLOT * SUITE_PORT_RANGE;

/**
 * Calculate unique ports for a test based on worker index and parallel index.
 * Each test gets a range of 10 ports to avoid collisions.
 * parallelIndex is unique within a worker for parallel tests.
 */
function getPortsForTest(workerIndex: number, parallelIndex: number) {
  // With 10 workers max and 10 parallel tests per worker max, this gives us
  // 100 unique web ports starting from BASE_PORT.
  const webserver = BASE_PORT + (workerIndex * 10 + parallelIndex) * PORT_STRIDE;
  return {
    webserver
  };
}

/**
 * Wait for the server to be ready by polling the readiness endpoint.
 * This verifies both NATS connectivity and JetStream initialization.
 */
async function waitForServer(
  baseURL: string,
  process: ChildProcess,
  logPath: string,
  timeoutMs = 45000
): Promise<void> {
  const start = Date.now();
  const url = new URL('/readyz', baseURL);

  while (Date.now() - start < timeoutMs) {
    if (process.exitCode !== null) {
      throw new Error(
        `Server exited with code ${process.exitCode} before becoming ready; see ${logPath}`
      );
    }
    try {
      const response = await fetch(url);
      if (response.ok) {
        const data = await response.json();
        if (data.status === 'ready') return;
      }
    } catch {
      // Server not ready yet
    }
    await new Promise((r) => setTimeout(r, 25));
  }
  throw new Error(
    `Server at ${baseURL} did not become ready within ${timeoutMs}ms; see ${logPath}`
  );
}

export interface StartServerOptions {
  /** Additional environment variables for the server process */
  env?: Record<string, string>;
  /** Chatto executable to launch. Defaults to the test-tagged E2E binary. */
  executablePath?: string;
  /** Stable suffix used to isolate this process's data, logs, and operator socket. */
  instanceId?: string;
  /** Port offset for additional servers started by the same Playwright test. */
  portOffset?: number;
  /** Hostname advertised to the browser. The server still listens on its configured port. */
  hostname?: string;
  /** Enable the local production operator API and expose its socket in ServerInfo. */
  operatorApi?: boolean;
  /** Enable the consumer search API and bundled Bleve provider for this test. */
  searchProvider?: boolean;
}

function safePathSegment(value: string): string {
  return value.replace(/[^a-zA-Z0-9-]/g, '-');
}

/**
 * Spawns a Chatto server for a specific test.
 * Uses environment variables to override ports.
 */
export async function startServer(
  testInfo: TestInfo,
  options: StartServerOptions = {}
): Promise<ServerInfo> {
  const ports = getPortsForTest(
    testInfo.workerIndex,
    testInfo.parallelIndex + (options.portOffset ?? 0)
  );
  const instanceId = safePathSegment(options.instanceId ?? 'primary');
  const testId = safePathSegment(testInfo.testId);
  const dataDir = path.join(__dirname, `data-${testId}-${instanceId}`);
  const searchDirectory = options.searchProvider ? `${dataDir}-search` : undefined;
  const executablePath = options.executablePath ?? path.join(__dirname, 'bin', 'chatto');
  const hostname = options.hostname ?? 'localhost';
  const baseURL = `http://${hostname}:${ports.webserver}`;
  const logPath = testInfo.outputPath(`${instanceId}-server.log`);
  // Unix-domain socket paths are short (roughly 100 bytes on macOS/Linux), so
  // keep operator sockets out of the potentially long workspace/test path.
  const operatorDir = options.operatorApi
    ? mkdtempSync(path.join(os.tmpdir(), 'chatto-e2e-operator-'))
    : undefined;
  const operatorSocketPath = operatorDir ? path.join(operatorDir, 'operator.sock') : undefined;

  if (existsSync(dataDir)) {
    rmSync(dataDir, { recursive: true });
  }
  mkdirSync(dataDir, { recursive: true });
  mkdirSync(path.dirname(logPath), { recursive: true });
  const logStream = createWriteStream(logPath, { flags: 'w' });

  if (operatorDir) {
    chmodSync(operatorDir, 0o700);
  }

  // The default test binary honors chatto.toml's bootstrap section. Production
  // binaries ignore it and can instead be provisioned through the operator API.
  const serverProcess = spawn(executablePath, ['start', '-c', 'chatto.toml'], {
    cwd: __dirname,
    env: {
      ...process.env,
      CHATTO_VIDEO_ENABLED: 'false',
      CHATTO_TEST_EMAIL_ENDPOINT: 'true',
      ...options.env,
      ...(searchDirectory
        ? {
            CHATTO_SEARCH_ENABLED: 'true',
            CHATTO_SEARCH_PROVIDER_ENABLED: 'true',
            CHATTO_SEARCH_PROVIDER_DIRECTORY: searchDirectory,
            CHATTO_SEARCH_PROVIDER_LANGUAGES: 'en'
          }
        : {}),
      CHATTO_WEBSERVER_PORT: String(ports.webserver),
      CHATTO_WEBSERVER_URL: baseURL,
      CHATTO_NATS_EMBEDDED_PORT: '0',
      CHATTO_NATS_EMBEDDED_HTTP_PORT: '0',
      CHATTO_NATS_EMBEDDED_DATA_DIR: dataDir,
      ...(operatorSocketPath
        ? {
            CHATTO_OPERATOR_API_ENABLED: 'true',
            CHATTO_OPERATOR_API_SOCKET_PATH: operatorSocketPath
          }
        : {})
    },
    stdio: ['ignore', 'pipe', 'pipe']
  });

  const prefix = `[${testInfo.title}:${instanceId}]`;
  serverProcess.stdout?.on('data', (data) => {
    logStream.write(data);
    if (process.env.DEBUG_E2E) {
      console.log(`${prefix} ${data.toString().trim()}`);
    }
  });
  serverProcess.stderr?.on('data', (data) => {
    logStream.write(data);
    if (process.env.DEBUG_E2E) {
      console.error(`${prefix} ${data.toString().trim()}`);
    }
  });
  serverProcess.on('close', () => logStream.end());

  const server = {
    baseURL,
    port: ports.webserver,
    process: serverProcess,
    executablePath,
    dataDir,
    searchDirectory,
    logPath,
    operatorSocketPath
  };
  try {
    await waitForServer(baseURL, serverProcess, logPath);
  } catch (error) {
    await stopServer(server);
    throw error;
  }

  return server;
}

/**
 * Stops a Chatto server and cleans up its data directory.
 */
export async function stopServer(server: ServerInfo, _testInfo?: TestInfo): Promise<void> {
  // Kill the server process
  if (server.process.exitCode === null) {
    server.process.kill('SIGTERM');
  } else {
    cleanupServerDirectories(server);
    return;
  }

  // Wait for process to exit
  await new Promise<void>((resolve) => {
    const timeout = setTimeout(() => {
      server.process.kill('SIGKILL');
      resolve();
    }, 5000);

    server.process.once('exit', () => {
      clearTimeout(timeout);
      resolve();
    });
  });

  cleanupServerDirectories(server);
}

function cleanupServerDirectories(server: ServerInfo): void {
  if (existsSync(server.dataDir)) {
    rmSync(server.dataDir, { recursive: true });
  }
  if (server.searchDirectory && existsSync(server.searchDirectory)) {
    rmSync(server.searchDirectory, { recursive: true });
  }
  if (server.operatorSocketPath) {
    const operatorDir = path.dirname(server.operatorSocketPath);
    if (existsSync(operatorDir)) {
      rmSync(operatorDir, { recursive: true });
    }
  }
}
