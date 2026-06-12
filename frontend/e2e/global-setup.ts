import { execSync } from 'child_process';
import { existsSync } from 'fs';

/**
 * Global setup runs once before all tests. Local runs invoke
 * `mise build-e2e-server` so iterating on backend + e2e together doesn't
 * silently use a stale binary. CI shards can opt out after downloading the
 * binary built by the dedicated E2E build job.
 */
export default function globalSetup() {
  if (process.env.CHATTO_E2E_SKIP_SERVER_BUILD === '1') {
    const binaryPath = 'e2e/fixtures/bin/chatto';
    if (!existsSync(binaryPath)) {
      throw new Error(
        `CHATTO_E2E_SKIP_SERVER_BUILD=1 but ${binaryPath} is missing. ` +
          'Download the E2E server artifact before running Playwright.'
      );
    }
    return;
  }

  execSync('mise build-e2e-server', { stdio: 'inherit', cwd: process.cwd() });
}
