// Tests must not create private temporary directories or change TMPDIR for the test process.
import { vi } from 'vitest';

vi.mock('./private-temp.ts', () => ({ usePrivateTempDirectory: async () => '' }));
