/** Test merging without Pi storage, provider setup, or a network. */
import { expect, test } from 'vitest';
import type { ContextEvent } from '@earendil-works/pi-coding-agent';
import { mergeThreadContext, type ThreadContext } from './context-messages.ts';

type Messages = ContextEvent['messages'];
const user = (content: string): Messages[number] => ({ role: 'user', content, timestamp: 1 });

test('merges IDs and edits without mutating inputs or confusing identical human text', () => {
  const history: Messages = [
    user(
      JSON.stringify({
        currentMessages: [
          {
            id: 'one',
            author: 'human',
            text: 'Old body',
            inReplyTo: { id: 'target', author: 'bot' }
          },
          { id: 'outside', text: 'Older context' }
        ]
      })
    ),
    user(JSON.stringify({ currentMessages: [{ id: 'two', text: 'New request' }] }))
  ];
  const snapshot: ThreadContext = {
    messages: [
      { id: 'one', author: 'human', text: 'Edited body' },
      { id: 'copy', author: 'other', text: 'Edited body' },
      { id: 'two', text: 'New request' }
    ],
    olderContextOmitted: false
  };
  const original = structuredClone({ history, snapshot });
  const result = mergeThreadContext(history, snapshot);
  const encoded = JSON.stringify(result);
  expect(encoded).not.toContain('Old body');
  expect(encoded.match(/Edited body/g)).toHaveLength(2);
  expect(encoded.match(/New request/g)).toHaveLength(1);
  expect(encoded).toContain('Older context');
  expect(encoded).toContain('inReplyTo');
  expect(encoded).toContain('target');
  expect({ history, snapshot }).toEqual(original);
  expect(mergeThreadContext(history, snapshot)).toEqual(result);
});

test.each(['ordinary text', '{bad json', 'null', '{"currentMessages":false}'])(
  'keeps unrecognized prompts intact: %s',
  (content) => {
    const request = mergeThreadContext([user(content)], {
      messages: [],
      olderContextOmitted: true
    });
    const last = request[0];
    expect(last?.role === 'user' && last.content).toEqual([
      expect.objectContaining({ text: expect.stringContaining('"olderContextOmitted":true') }),
      { type: 'text', text: content }
    ]);
  }
);

test('supplies messages absent from retained history after compaction', () => {
  const snapshot: ThreadContext = {
    messages: [{ id: 'one', text: 'Recent fact' }],
    olderContextOmitted: false
  };
  const result = mergeThreadContext([user('Compacted summary'), user('Current request')], snapshot);
  expect(result[0]).toEqual(user('Compacted summary'));
  expect(JSON.stringify(result).match(/Recent fact/g)).toHaveLength(1);
});
