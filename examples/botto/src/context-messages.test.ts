/** Test merging without Pi storage, provider setup, or a network. */
import { expect, test } from 'vitest';
import type { ContextEvent } from '@earendil-works/pi-coding-agent';
import { mergeThreadContext, type ThreadContext } from './context-messages.ts';

type Messages = ContextEvent['messages'];
const user = (content: string): Messages[number] => ({ role: 'user', content, timestamp: 1 });

test('keeps successive screenshots with their original message batches', () => {
  const history = ['first', 'second'].map((id) =>
    user(
      JSON.stringify({
        currentMessages: [{ id, text: id === 'first' ? "What's this?" : 'And this?' }]
      })
    )
  );
  const snapshot: ThreadContext = {
    messages: [
      { id: 'first', text: "What's this?" },
      { id: 'second', text: 'And this?' }
    ],
    olderContextOmitted: false,
    // Loading order prioritizes the newest image, not conversation order.
    images: ['second', 'first'].map((id) => ({
      messageId: id,
      attachmentId: `${id}-attachment`,
      data: `${id}-pixels`,
      mimeType: 'image/png'
    }))
  };
  const original = structuredClone({ history, snapshot });
  const result = mergeThreadContext(history, snapshot);
  expect(JSON.stringify(result[0])).toContain('first-pixels');
  expect(JSON.stringify(result[0])).not.toContain('second-pixels');
  expect(JSON.stringify(result[1])).toContain('second-pixels');
  expect(JSON.stringify(result[1])).not.toContain('first-pixels');
  expect({ history, snapshot }).toEqual(original);
  expect(mergeThreadContext(history, snapshot)).toEqual(result);

  // After compaction, recover the older image with an explicit context label.
  const compacted = mergeThreadContext([history[1]!], snapshot);
  expect(JSON.stringify(compacted)).toContain('first-pixels');
  expect(JSON.stringify(compacted)).toContain('from additional thread context');
});

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

test('keeps the current participant directory even when all messages are already represented', () => {
  const history = [
    user(JSON.stringify({ currentMessages: [{ id: 'one', author: 'human', text: 'Hello' }] }))
  ];
  const snapshot: ThreadContext = {
    messages: [{ id: 'one', author: 'human', text: 'Hello' }],
    olderContextOmitted: false,
    participants: [{ id: 'human', username: 'cedar', displayName: 'Cedar' }]
  };
  const merged = JSON.stringify(mergeThreadContext(history, snapshot));
  expect(merged).toContain('participants');
  expect(merged).toContain('cedar');
  expect(merged).toContain('Cedar');
});
