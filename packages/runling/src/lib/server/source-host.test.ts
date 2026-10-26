import { afterEach, expect, test, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  load: vi.fn(),
  subscribe: vi.fn(),
  configClose: vi.fn(),
  replace: vi.fn(),
  managerClose: vi.fn(),
  start: vi.fn(),
  storeClose: vi.fn(),
  restore: vi.fn()
}));
vi.mock('./web-config.ts', () => ({
  getConfigReloader: () => ({
    load: mocks.load,
    subscribe: mocks.subscribe,
    close: mocks.configClose,
    get revision() {
      return mocks.load.mock.calls.length;
    }
  })
}));
vi.mock('./run-store.ts', () => ({
  getRunStore: async () => ({ start: mocks.start, close: mocks.storeClose, restore: mocks.restore })
}));
vi.mock('runling/server', () => ({
  SourceManager: class {
    replace = mocks.replace;
    close = mocks.managerClose;
  }
}));
import { startSourceHost, stopSourceHost } from './source-host.ts';

afterEach(async () => {
  await stopSourceHost();
  vi.resetAllMocks();
});

test('startup and reload restore registered roots before launching sources', async () => {
  mocks.load.mockResolvedValue({ webhooks: {} });
  mocks.subscribe.mockReturnValue(() => {});
  await startSourceHost();
  await startSourceHost();
  expect(mocks.replace).toHaveBeenCalledOnce();
  expect(mocks.restore).toHaveBeenCalledOnce();
  expect(mocks.restore.mock.invocationCallOrder[0]).toBeLessThan(
    mocks.replace.mock.invocationCallOrder[0]!
  );
  expect(mocks.start).not.toHaveBeenCalled();
  const listener = mocks.subscribe.mock.calls[0]?.[0];
  expect(listener).toBeTypeOf('function');
  listener();
  await vi.waitFor(() => expect(mocks.replace).toHaveBeenCalledTimes(2));
  expect(mocks.start).not.toHaveBeenCalled();
  await stopSourceHost();
  expect(mocks.managerClose).toHaveBeenCalledOnce();
  expect(mocks.storeClose).toHaveBeenCalledOnce();
});
