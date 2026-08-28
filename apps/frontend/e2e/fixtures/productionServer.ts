import { spawn } from 'child_process';
import type { TestInfo } from '@playwright/test';
import { startServer, type ServerInfo } from './server';

export interface ProductionUser {
  login: string;
  displayName: string;
  password: string;
  roles?: string[];
}

/**
 * Start an unmodified production Chatto executable with an isolated embedded
 * NATS store and local operator socket.
 */
export function startProductionServer(
  testInfo: TestInfo,
  executablePath: string,
  instanceId: string,
  portOffset: number,
  hostname = 'localhost'
): Promise<ServerInfo> {
  return startServer(testInfo, {
    executablePath,
    hostname,
    instanceId,
    operatorApi: true,
    portOffset
  });
}

/** Create a test account through the production, Unix-socket-only operator API. */
export async function createProductionUser(
  server: ServerInfo,
  user: ProductionUser
): Promise<void> {
  if (!server.operatorSocketPath) {
    throw new Error('Production server was not started with its operator API enabled');
  }

  const args = [
    'operator',
    '--operator-socket',
    server.operatorSocketPath,
    'user',
    'create',
    '--login',
    user.login,
    '--display-name',
    user.displayName,
    '--password-stdin'
  ];
  for (const role of user.roles ?? ['owner']) {
    args.push('--role', role);
  }

  await new Promise<void>((resolve, reject) => {
    const child = spawn(server.executablePath, args, {
      env: process.env,
      stdio: ['pipe', 'pipe', 'pipe']
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });
    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });
    child.once('error', reject);
    child.once('close', (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(
        new Error(
          `chatto operator user create exited with code ${code}: ${stderr.trim() || stdout.trim()}`
        )
      );
    });
    child.stdin.end(`${user.password}\n`);
  });
}
