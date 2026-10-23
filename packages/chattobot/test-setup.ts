// Fake conversation and worker agents do not need a scratch working directory.
// Classifier internals and real agent sessions still use Runling's private storage.
import { vi } from 'vitest';

vi.mock('runling', async (original) => ({
  ...(await original<typeof import('runling')>()),
  usePrivateTempDirectory: async () => ''
}));
