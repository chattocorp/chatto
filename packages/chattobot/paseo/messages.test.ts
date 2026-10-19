import { expect, test } from 'vitest';
import { createMessageAssembler } from './messages.ts';

test('tool activity separates assistant segments without exposing its details', () => {
  const output: string[] = [];
  const messages = createMessageAssembler((text) => output.push(text));
  const text = (text: string) =>
    messages.accept({
      type: 'timeline',
      provider: 'test',
      item: { type: 'assistant_message', text }
    });
  text('Running ');
  text('tests.');
  messages.accept({
    type: 'timeline',
    provider: 'test',
    item: {
      type: 'tool_call',
      callId: 'test',
      name: 'shell',
      status: 'running',
      error: null,
      detail: { type: 'shell', command: 'private command' }
    }
  });
  expect(output).toEqual(['Running tests.']);
  text('Done.');
  messages.flush();
  messages.flush();
  text('  ');
  messages.flush();
  expect(output).toEqual(['Running tests.', 'Done.']);
});
