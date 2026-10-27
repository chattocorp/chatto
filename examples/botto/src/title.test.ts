/** Check naming ownership, retained titles, input bounds, and non-fatal failures. */
import { beforeEach, expect, test, vi } from 'vitest';
import { createTaskContext } from 'runling';
import { createTitleWriter } from './title.ts';
import { parseSettings } from './settings.ts';

const mock = vi.hoisted(() => ({ describe: vi.fn() }));
vi.mock('runling', async (original) => ({
  ...(await original<typeof import('runling')>()),
  createDescription: () => mock.describe
}));
beforeEach(() => mock.describe.mockReset().mockResolvedValue('Weather forecast'));
const settings = parseSettings(
  '[chatto]\nurl="https://chat.example.com"\napi_key="unused"\n[agent]\nmodel="test/model"',
  'botto.toml'
);

test('names an untitled run once and skips unrecorded, empty, and already named inputs', async () => {
  const write = createTitleWriter(settings);
  const ctx = createTaskContext();
  const signal = new AbortController().signal;
  await write(ctx, 'weather', signal);
  const run = {
    id: 'run',
    title: undefined as string | undefined,
    setTitle: vi.fn((title: string) => {
      run.title = title;
    })
  };
  await write({ ...ctx, run }, ' ', signal);
  expect(mock.describe).not.toHaveBeenCalled();
  await write({ ...ctx, run }, 'x'.repeat(9000), signal);
  expect(mock.describe.mock.calls[0]![1]).toHaveLength(8000);
  expect(run.setTitle).toHaveBeenCalledExactlyOnceWith('Weather forecast');
  await write({ ...ctx, run }, 'different topic', signal);
  expect(mock.describe).toHaveBeenCalledOnce();
});

test('empty descriptions, failures, and cancelled late results do not set a title', async () => {
  const write = createTitleWriter(settings);
  const controller = new AbortController();
  const setTitle = vi.fn();
  const ctx = { ...createTaskContext(), run: { id: 'run', setTitle } };
  mock.describe.mockResolvedValueOnce('');
  await write(ctx, 'hello', controller.signal);
  mock.describe.mockRejectedValueOnce(new Error('private provider error'));
  await expect(write(ctx, 'question', controller.signal)).resolves.toBeUndefined();
  mock.describe.mockImplementationOnce(async () => {
    controller.abort();
    return 'Late';
  });
  await write(ctx, 'question', controller.signal);
  expect(setTitle).not.toHaveBeenCalled();
});
