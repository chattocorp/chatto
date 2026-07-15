import { describe, expect, it } from 'vitest';
import {
  canonicalServerOrigin,
  generateServerId,
  portableMetadataUpdate,
  recordPendingHomeMove,
  wasRemovedSinceLastSync
} from './serverIdentity';

describe('canonicalServerOrigin', () => {
  it('uses the URL origin rather than its path', () => {
    expect(canonicalServerOrigin('https://chat.example.com/some/path')).toBe(
      'https://chat.example.com'
    );
  });

  it('keeps different origins distinct', () => {
    expect(canonicalServerOrigin('https://trusted.example.com')).not.toBe(
      canonicalServerOrigin('https://attacker.example.com')
    );
  });
});

describe('generateServerId', () => {
  it('generates a local ID and resolves collisions', () => {
    expect(generateServerId('https://chat.example.com')).toBe('chat-example-com');
    expect(generateServerId('https://chat.example.com', ['chat-example-com'])).toBe(
      'chat-example-com-2'
    );
  });
});

describe('portableMetadataUpdate', () => {
  it('cannot replace a local origin or bearer token', () => {
    const update = portableMetadataUpdate({
      name: 'Renamed',
      iconUrl: 'https://assets.example.com/icon.png'
    });

    expect(update).toEqual({
      name: 'Renamed',
      iconUrl: 'https://assets.example.com/icon.png'
    });
    expect(update).not.toHaveProperty('url');
    expect(update).not.toHaveProperty('token');
  });
});

describe('wasRemovedSinceLastSync', () => {
  it('does not turn a first sync into a deletion', () => {
    expect(wasRemovedSinceLastSync('https://chat.example.com', new Set(), new Set())).toBe(false);
  });

  it('detects an entry removed after a successful baseline', () => {
    expect(
      wasRemovedSinceLastSync(
        'https://chat.example.com',
        new Set(['https://chat.example.com']),
        new Set()
      )
    ).toBe(true);
  });
});

describe('recordPendingHomeMove', () => {
  it('collapses a chain onto the newest home', () => {
    const moves = recordPendingHomeMove({}, 'https://a.example', 'https://b.example', 'UA');
    expect(recordPendingHomeMove(moves, 'https://b.example', 'https://c.example', 'UB')).toEqual({
      'https://a.example': { newOrigin: 'https://c.example', previousUserId: 'UA' },
      'https://b.example': { newOrigin: 'https://c.example', previousUserId: 'UB' }
    });
  });

  it('does not leave a cycle when a move is reversed', () => {
    const moves = recordPendingHomeMove({}, 'https://a.example', 'https://b.example', 'UA');
    expect(recordPendingHomeMove(moves, 'https://b.example', 'https://a.example', 'UB')).toEqual({
      'https://b.example': { newOrigin: 'https://a.example', previousUserId: 'UB' }
    });
  });
});
