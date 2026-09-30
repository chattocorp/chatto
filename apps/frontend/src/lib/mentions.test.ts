import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
/**
 * Unit tests for mention parsing utilities (pure functions).
 * Tests for resolveRenderedMentions are in mentions.svelte.test.ts (requires browser APIs).
 */
import { describe, it, expect } from 'vitest';
import {
  extractMentions,
  findMemberByMention,
  hasRoleOrVirtualMention,
  isUserMentioned,
  type RoomMember
} from './mentions';

// Helper to create test members
function member(login: string, displayName?: string): RoomMember {
  return {
    id: login,
    login,
    displayName: displayName ?? login,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE
  };
}

// The shared cases in testdata/mentions/extraction.json cover the parsing
// rules in detail (see markdownMentions.test.ts).
describe('extractMentions', () => {
  it('returns deduplicated handles in order of appearance', () => {
    expect(extractMentions('@bob and @alice and @bob')).toEqual(['bob', 'alice']);
  });

  it('ignores handles in URLs, code, blockquotes, and links', () => {
    expect(
      extractMentions(
        'https://social.5f9.de/@carol/1 `@dora` [@erin](https://example.com)\n> @frank\n\n@alice/@bob'
      )
    ).toEqual(['alice', 'bob']);
  });

  it('returns an empty array without mentions', () => {
    expect(extractMentions('')).toEqual([]);
    expect(extractMentions('user@example.com')).toEqual([]);
  });
});

describe('findMemberByMention', () => {
  const members = [member('alice', 'Alice Smith'), member('bob', 'Bob Jones')];

  it('finds member by exact login match (case-insensitive)', () => {
    expect(findMemberByMention('alice', members)?.login).toBe('alice');
    expect(findMemberByMention('ALICE', members)?.login).toBe('alice');
  });

  it('finds member by display name (case-insensitive)', () => {
    expect(findMemberByMention('Alice Smith', members)?.login).toBe('alice');
    expect(findMemberByMention('alice smith', members)?.login).toBe('alice');
  });

  it('returns undefined for non-existent username', () => {
    expect(findMemberByMention('charlie', members)).toBeUndefined();
  });

  it('returns undefined for empty members list', () => {
    expect(findMemberByMention('alice', [])).toBeUndefined();
  });
});

describe('isUserMentioned', () => {
  const members = [member('alice', 'Alice Smith'), member('bob', 'Bob Jones')];

  it('returns false when the user handle only appears in a URL', () => {
    expect(isUserMentioned('https://social.5f9.de/@alice/1', 'alice', [member('alice')])).toBe(
      false
    );
  });

  it('returns true when user is mentioned by login', () => {
    expect(isUserMentioned('Hello @alice!', 'alice', members)).toBe(true);
  });

  it('extracts partial username from spaced display name mention', () => {
    // "@Alice Smith" extracts "Alice" which DOES match alice's login (case-insensitive)
    // This is expected behavior - the regex can't capture spaces in usernames
    expect(isUserMentioned('Hello @Alice Smith!', 'alice', members)).toBe(true);
  });

  it('returns false when partial mention does not match any member', () => {
    // "@Charlie Brown" extracts "Charlie" which doesn't match any member
    expect(isUserMentioned('Hello @Charlie Brown!', 'charlie', members)).toBe(false);
  });

  it('returns false when user is not mentioned', () => {
    expect(isUserMentioned('Hello @bob!', 'alice', members)).toBe(false);
  });

  it('returns false when mentioned username is not a valid member', () => {
    expect(isUserMentioned('Hello @charlie!', 'charlie', members)).toBe(false);
  });

  it('is case-insensitive for user login', () => {
    expect(isUserMentioned('Hello @ALICE!', 'alice', members)).toBe(true);
  });

  it('returns false for empty text', () => {
    expect(isUserMentioned('', 'alice', members)).toBe(false);
  });

  it('returns false when the mention handle is split by emphasis', () => {
    expect(isUserMentioned('@al*ice*', 'alice', members)).toBe(false);
    expect(isUserMentioned('@*alice*', 'alice', members)).toBe(false);
  });

  it('returns false for a mention inside inline code', () => {
    expect(isUserMentioned('Hello `@alice`!', 'alice', members)).toBe(false);
  });

  it('returns false for a mention inside escaped-backtick inline code', () => {
    expect(isUserMentioned('Hello \\`@alice\\`!', 'alice', members)).toBe(false);
  });

  it('returns true for a mention immediately after inline code', () => {
    expect(isUserMentioned('Hello `cmd`@alice!', 'alice', members)).toBe(true);
  });

  it('returns false for a mention inside a blockquote', () => {
    expect(isUserMentioned('> Hello @alice!', 'alice', members)).toBe(false);
  });
});

describe('hasRoleOrVirtualMention', () => {
  it('ignores role and virtual handles in URLs', () => {
    expect(hasRoleOrVirtualMention('https://social.5f9.de/@all/1', ['moderator'])).toBe(false);
    expect(hasRoleOrVirtualMention('www.example.com/@moderator', ['moderator'])).toBe(false);
  });

  it('returns true for virtual room mentions', () => {
    expect(hasRoleOrVirtualMention('@all please read', [])).toBe(true);
    expect(hasRoleOrVirtualMention('@HERE please read', [])).toBe(true);
  });

  it('returns true for known role handles case-insensitively', () => {
    expect(hasRoleOrVirtualMention('Heads up @Mods', ['mods'])).toBe(true);
  });

  it('returns false for ordinary user mentions and unknown handles', () => {
    expect(hasRoleOrVirtualMention('Hi @alice and @unknown', ['mods'])).toBe(false);
  });

  it('ignores role and virtual mentions in code and blockquotes', () => {
    expect(hasRoleOrVirtualMention('`@mods` @alice\n> @all', ['mods'])).toBe(false);
    expect(hasRoleOrVirtualMention('```\n@all\n```\n@mods', ['mods'])).toBe(true);
  });
});
