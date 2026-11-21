/** Verify model-selected content cannot bypass the channel length limit or terminal result. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { expect, test, vi } from 'vitest';
import { announcementResponse } from './announcement-response.ts';
async function setup() {
  const controller = new AbortController();
  const response = announcementResponse(controller.signal);
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  const on = vi.fn();
  await response.extension({ registerTool, on } as unknown as ExtensionAPI);
  const call = (summary: string, details = 'Details') =>
    registerTool.mock.calls[0]![0].execute(
      'call',
      { summary, details },
      undefined,
      undefined,
      {} as ExtensionToolContext
    );
  return { response, controller, call, on };
}
test('requires a terminal result and blocks later actions', async () => {
  const { response, call, on } = await setup();
  expect(() => response.result()).toThrow('did not finish');
  expect(await call('Short summary')).toMatchObject({ terminate: true });
  expect(response.result()).toEqual({ summary: 'Short summary', details: 'Details' });
  expect(on.mock.calls.find(([name]) => name === 'tool_call')![1]()).toMatchObject({
    block: true,
    terminate: true
  });
  await expect(call('Second summary')).rejects.toThrow('already completed');
});
test.each(['', 'x'.repeat(401)])('rejects invalid channel text', async (summary) => {
  const { call, response } = await setup();
  await expect(call(summary)).rejects.toThrow('short Markdown');
  expect(() => response.result()).toThrow();
});
test('rejects excessive details and cancellation', async () => {
  const { call, controller } = await setup();
  await expect(call('Summary', 'x'.repeat(4001))).rejects.toThrow();
  controller.abort();
  await expect(call('Summary')).rejects.toThrow();
});

test('preserves a compact Markdown heading and summary within the existing limit', async () => {
  const { call, response } = await setup();
  const summary =
    '**Landed on `main` · Authling**\n\nThe admin user directory now supports sorting and infinite scroll.';
  await call(summary);
  expect(response.result()!.summary).toBe(summary);
  expect(response.result()!.summary.length).toBeLessThanOrEqual(400);
});

test('follow-up-only sessions expose no channel announcement tool and terminate with null', async () => {
  const response = announcementResponse(new AbortController().signal, false);
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  const on = vi.fn();
  await response.extension({ registerTool, on } as unknown as ExtensionAPI);
  expect(registerTool.mock.calls.map(([tool]) => tool.name)).toEqual(['finish_followups']);
  const finish = registerTool.mock.calls[0]![0];
  await finish.execute('call', {}, undefined, undefined, {} as ExtensionToolContext);
  expect(response.result()).toBeNull();
  expect(on.mock.calls.find(([name]) => name === 'tool_call')![1]()).toMatchObject({
    block: true,
    terminate: true
  });
  await expect(
    finish.execute('call', {}, undefined, undefined, {} as ExtensionToolContext)
  ).rejects.toThrow('already completed');
});
