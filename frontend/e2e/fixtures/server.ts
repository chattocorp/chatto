import { spawn, type ChildProcess } from 'child_process';
import { existsSync, mkdirSync, rmSync } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import type { TestInfo } from '@playwright/test';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export interface ServerInfo {
  baseURL: string;
  port: number;
  natsPort: number;
  process: ChildProcess;
}

const PORT_STRIDE = 10;

// Random offset for this test suite run to avoid port collisions
// when running multiple test suites simultaneously.
// Each suite needs ~100 ports (10 workers × 10 stride).
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
  // With 10 workers max and 10 parallel tests per worker max,
  // this gives us 100 unique port ranges starting from BASE_PORT
  const base = BASE_PORT + (workerIndex * 10 + parallelIndex) * PORT_STRIDE;
  return {
    webserver: base,
    nats: base + 2,
    natsHttp: base + 3
  };
}

/**
 * Wait for the server to be ready by polling the readiness endpoint.
 * This verifies both NATS connectivity and JetStream initialization.
 */
async function waitForServer(port: number, timeoutMs = 45000): Promise<void> {
  const start = Date.now();
  const url = `http://localhost:${port}/readyz`;

  while (Date.now() - start < timeoutMs) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        const data = await response.json();
        if (data.status === 'ready') return;
      }
    } catch {
      // Server not ready yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`Server on port ${port} did not become ready within ${timeoutMs}ms`);
}

export interface StartServerOptions {
  /** Additional environment variables for the server process */
  env?: Record<string, string>;
}

/**
 * Spawns a Chatto server for a specific test.
 * Uses environment variables to override ports.
 */
export async function startServer(
  testInfo: TestInfo,
  options: StartServerOptions = {}
): Promise<ServerInfo> {
  const ports = getPortsForTest(testInfo.workerIndex, testInfo.parallelIndex);
  // Use testId for unique data directory per test
  const dataDir = path.join(__dirname, `data-${testInfo.testId.replace(/[^a-zA-Z0-9]/g, '-')}`);

  // Clean up and create data directory
  if (existsSync(dataDir)) {
    rmSync(dataDir, { recursive: true });
  }
  mkdirSync(dataDir, { recursive: true });

  // chatto.toml seeds the e2eadmin user via [bootstrap] on every server start.
  const serverProcess = spawn(path.join(__dirname, 'bin', 'chatto'), ['start', '-c', 'chatto.toml'], {
    cwd: __dirname,
    env: {
      ...process.env,
      ...options.env,
      CHATTO_WEBSERVER_PORT: String(ports.webserver),
      CHATTO_WEBSERVER_URL: `http://localhost:${ports.webserver}`,
      CHATTO_NATS_EMBEDDED_PORT: String(ports.nats),
      CHATTO_NATS_EMBEDDED_HTTP_PORT: String(ports.natsHttp),
      CHATTO_NATS_EMBEDDED_DATA_DIR: dataDir,
      CHATTO_TEST_EMAIL_ENDPOINT: 'true' // Enable test email endpoint for E2E tests
    },
    stdio: ['ignore', 'pipe', 'pipe']
  });

  // Log server output for debugging (prefix with test title)
  const prefix = `[${testInfo.title}]`;
  serverProcess.stdout?.on('data', (data) => {
    if (process.env.DEBUG_E2E) {
      console.log(`${prefix} ${data.toString().trim()}`);
    }
  });
  serverProcess.stderr?.on('data', (data) => {
    if (process.env.DEBUG_E2E) {
      console.error(`${prefix} ${data.toString().trim()}`);
    }
  });

  // Wait for server to be ready. The admin user is created during startup via
  // the [bootstrap] section in chatto.toml, so by the time the readiness check
  // passes the user exists.
  await waitForServer(ports.webserver);

  return {
    baseURL: `http://localhost:${ports.webserver}`,
    port: ports.webserver,
    natsPort: ports.nats,
    process: serverProcess
  };
}

/**
 * Stops a Chatto server and cleans up its data directory.
 */
export async function stopServer(server: ServerInfo, testInfo: TestInfo): Promise<void> {
  const dataDir = path.join(__dirname, `data-${testInfo.testId.replace(/[^a-zA-Z0-9]/g, '-')}`);

  // Kill the server process
  server.process.kill('SIGTERM');

  // Wait for process to exit
  await new Promise<void>((resolve) => {
    const timeout = setTimeout(() => {
      server.process.kill('SIGKILL');
      resolve();
    }, 5000);

    server.process.on('exit', () => {
      clearTimeout(timeout);
      resolve();
    });
  });

  // Clean up data directory
  if (existsSync(dataDir)) {
    rmSync(dataDir, { recursive: true });
  }
}
