/** Verify terminal choices and progress delivery without a model or Chatto connection. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { expect, test, vi } from 'vitest';
import { responseTools } from './response.ts';

async function setup(allowReaction = true, post = vi.fn(async (_text: string) => {})) {
  const controller = new AbortController();
  const response = responseTools(controller.signal, allowReaction, post);
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  const on = vi.fn();
  await response.extension({ registerTool, on } as unknown as ExtensionAPI);
  const call = (name: string, args: Record<string, unknown> = {}) => {
    const tool = registerTool.mock.calls.find(([tool]) => tool.name === name)?.[0];
    if (!tool) throw new Error('Missing tool');
    return tool.execute('call', args, undefined, undefined, {} as ExtensionToolContext);
  };
  return { response, controller, post, call, on, registerTool };
}

test.each(['stay_silent', 'react_to_message'])(
  'terminal %s suppresses text and blocks later tools',
  async (name) => {
    const { response, call, on, post } = await setup();
    expect(await call(name, name === 'stay_silent' ? {} : { emoji: '+1' })).toMatchObject({
      terminate: true
    });
    expect(response.result('Conflicting assistant text')).toEqual(
      name === 'stay_silent' ? { action: 'silent' } : { action: 'react', emoji: '+1' }
    );
    expect(on.mock.calls.find(([event]) => event === 'tool_call')![1]()).toMatchObject({
      block: true,
      terminate: true
    });
    expect(post).not.toHaveBeenCalled();
  }
);

test('reaction tool is absent for incoming reactions; accidental empty text fails', async () => {
  const { registerTool, response } = await setup(false);
  expect(registerTool.mock.calls.map(([tool]) => tool.name)).toEqual([
    'stay_silent',
    'post_update'
  ]);
  expect(() => response.result(' ')).toThrow('explicit response decision');
  expect(response.result(' Answer ')).toEqual({ action: 'reply', text: 'Answer' });
});

test('progress waits for delivery and permits an explicit silent ending', async () => {
  const delivery = Promise.withResolvers<void>();
  const post = vi.fn((_text: string) => delivery.promise);
  const { call, response } = await setup(true, post);
  let finished = false;
  const pending = Promise.resolve(call('post_update', { text: ' Working on it. ' })).then(() => {
    finished = true;
  });
  await Promise.resolve();
  expect(finished).toBe(false);
  expect(post).toHaveBeenCalledExactlyOnceWith('Working on it.');
  delivery.resolve();
  await pending;
  await call('stay_silent');
  expect(response.result('')).toEqual({ action: 'silent' });
});

test('failed progress does not report success and cancellation prevents final choices', async () => {
  const post = vi.fn(async (_text: string) => {
    throw new Error('Uncertain delivery');
  });
  const { call, controller, response } = await setup(true, post);
  await expect(call('post_update', { text: 'Working' })).rejects.toThrow('Uncertain delivery');
  expect(post).toHaveBeenCalledOnce();
  controller.abort();
  await expect(call('stay_silent')).rejects.toThrow();
  expect(() => response.result('Answer')).toThrow();
});
