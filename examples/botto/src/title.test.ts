/** Verify root-only title registration and cancellation without a model call. */
import { expect, test, vi } from 'vitest';
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { createTaskContext } from 'runling';
import { titleExtension } from './title.ts';

test('registers only for a recorded root and forwards titles without posting messages', async () => {
  const ctx = createTaskContext();
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  const api = { registerTool } as unknown as ExtensionAPI;
  const controller = new AbortController();
  await titleExtension(ctx, controller.signal)(api);
  expect(registerTool).not.toHaveBeenCalled();
  const setTitle = vi.fn();
  await titleExtension({ ...ctx, run: { id: 'run', setTitle } }, controller.signal)(api);
  const tool = registerTool.mock.calls[0]![0];
  expect(tool.name).toBe('set_conversation_title');
  const call = () =>
    tool.execute(
      'call',
      { title: 'Settings storage' },
      undefined,
      undefined,
      {} as ExtensionToolContext
    );
  await call();
  expect(setTitle).toHaveBeenCalledExactlyOnceWith('Settings storage');
  controller.abort();
  await expect(call()).rejects.toThrow();
  expect(setTitle).toHaveBeenCalledTimes(1);
});
