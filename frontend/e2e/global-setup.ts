import { execSync } from 'child_process';

/**
 * Global setup runs once before all tests. Always invokes
 * `mise build-e2e-server` — mise's source/output tracking turns this
 * into a no-op when nothing has changed and a real rebuild when backend
 * code has, so iterating on backend + e2e together doesn't silently use
 * a stale binary.
 */
export default function globalSetup() {
  execSync('mise build-e2e-server', { stdio: 'inherit', cwd: process.cwd() });
}
