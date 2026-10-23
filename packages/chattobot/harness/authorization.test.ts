/** Reaction approvals use the same current-turn identity gate as text requests. */
import { expect, test } from 'vitest';
import { requireMaintainerRequest } from './authorization.ts';
import type { IncomingMessage } from '../chatto/message.ts';

test.each(['GitHub', 'Investigation', 'Paseo'] as const)(
  '%s accepts only the current maintainer reaction event',
  (capability) => {
    const reaction: IncomingMessage = {
      id: 'reaction',
      authorId: 'maintainer',
      roomId: 'room',
      threadRootId: 'root',
      body: '+1',
      reaction: { emoji: '+1', messageId: 'proposal', body: 'Shall I do this?' }
    };
    const maintainers = new Set(['maintainer']);
    expect(requireMaintainerRequest(capability, 'reaction', [reaction], maintainers)).toBe(
      reaction
    );
    for (const [requestId, messages] of [
      ['proposal', [reaction]],
      ['reaction', []],
      ['reaction', [{ ...reaction, authorId: 'ordinary-user' }]],
      ['reaction', [{ ...reaction, trigger: 'follow-up' as const }]]
    ] as const)
      expect(() => requireMaintainerRequest(capability, requestId, messages, maintainers)).toThrow(
        'maintainer'
      );
  }
);
