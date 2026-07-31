import { execFileSync } from 'node:child_process';

// Build once before workers start. mise's source tracking keeps direct
// Playwright invocations from silently exercising a stale Authling binary.
export default function globalSetup(): void {
  if (process.env.AUTHLING_E2E_SKIP_BUILD === '1') return;
  execFileSync('mise', ['build'], { cwd: process.cwd(), stdio: 'inherit' });
}
