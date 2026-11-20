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

test.each([
  ['👏', 'clap'],
  [':clap:', 'clap'],
  ['clap', 'clap'],
  ['👍', 'thumbsup'],
  ['👏🏽', 'clap::skin-tone-4']
])('normalizes %s and terminates without posting either text response', async (emoji, expected) => {
  const { call, response, post, registerTool } = await setup();
  const tool = registerTool.mock.calls.find(([tool]) => tool.name === 'react_to_message')![0];
  const { Value } = await import('typebox/value');
  expect(Value.Check(tool.parameters, { emoji })).toBe(true);
  expect(await call('react_to_message', { emoji })).toMatchObject({ terminate: true });
  await expect(call('post_update', { text: 'Nice work!' })).rejects.toThrow('final response');
  expect(response.result('Congrats on shipping!')).toEqual({ action: 'react', emoji: expected });
  expect(post).not.toHaveBeenCalled();
});

test('an invalid reaction cannot produce a progress post followed by a final reply', async () => {
  const { call, response, post } = await setup();
  await expect(call('react_to_message', { emoji: '👏👏' })).rejects.toThrow('supported emoji');
  await expect(call('post_update', { text: 'Nice work!' })).rejects.toThrow(
    'cannot become a progress update'
  );
  expect(post).not.toHaveBeenCalled();
  expect(response.result('Nice work!')).toEqual({ action: 'reply', text: 'Nice work!' });
});

test('a rejected reaction can be corrected without extra messages', async () => {
  const { call, response, post } = await setup();
  await expect(call('react_to_message', { emoji: 'not-an-emoji' })).rejects.toThrow();
  await call('react_to_message', { emoji: 'clap' });
  expect(response.result('')).toEqual({ action: 'react', emoji: 'clap' });
  expect(post).not.toHaveBeenCalled();
});
