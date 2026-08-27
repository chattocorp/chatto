import { execSync } from 'child_process';
import path from 'node:path';

/**
 * Global setup runs once before all tests. Always invokes
 * the Chatto E2E server and Authling binary. Mise's source/output tracking
 * turns either build into a no-op when nothing has changed, while preventing
 * cross-product browser tests from silently using stale binaries.
 */
export default function globalSetup() {
  execSync('mise build-e2e-server', { stdio: 'inherit', cwd: process.cwd() });
  execSync('mise build', {
    stdio: 'inherit',
    cwd: path.resolve(process.cwd(), '../../authling')
  });
}
