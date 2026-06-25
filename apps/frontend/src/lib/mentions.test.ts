/**
 * Unit tests for mention parsing utilities (pure functions).
 * Tests for wrapValidMentions are in mentions.svelte.test.ts (requires browser APIs).
 */
import { describe, it, expect } from 'vitest';
import { extractMentions, findMemberByMention, isUserMentioned, type RoomMember } from './mentions';
import type { PresenceStatus } from '$lib/gql/graphql';

// Helper to create test members
function member(login: string, displayName?: string): RoomMember {
  return {
    id: login,
    login,
    displayName: displayName ?? login,
    avatarUrl: null,
    presenceStatus: 'OFFLINE' as PresenceStatus
  };
}

describe('extractMentions', () => {
  it('extracts single mention', () => {
    expect(extractMentions('Hello @alice')).toEqual(['alice']);
  });

  it('extracts multiple mentions', () => {
    expect(extractMentions('Hey @alice and @bob')).toEqual(['alice', 'bob']);
  });

  it('deduplicates mentions', () => {
    expect(extractMentions('@alice @bob @alice')).toEqual(['alice', 'bob']);
  });

  it('handles mention at start of string', () => {
    expect(extractMentions('@alice hello')).toEqual(['alice']);
  });

  it('handles mention with punctuation before', () => {
    expect(extractMentions('Hi, @alice!')).toEqual(['alice']);
  });

  it('handles usernames with dots', () => {
    expect(extractMentions('@john.doe')).toEqual(['john.doe']);
  });

  it('handles usernames with hyphens and underscores', () => {
    expect(extractMentions('@user_name and @user-name')).toEqual(['user_name', 'user-name']);
  });

  it('does not match email addresses', () => {
    expect(extractMentions('email user@example.com')).toEqual([]);
  });

  it('returns empty array for text without mentions', () => {
    expect(extractMentions('Hello world')).toEqual([]);
  });

  it('returns empty array for empty string', () => {
    expect(extractMentions('')).toEqual([]);
  });

  it('handles @ symbol without valid username', () => {
    expect(extractMentions('@ alone')).toEqual([]);
  });

  it('does not include trailing dot in username', () => {
    // The regex doesn't allow trailing dots
    expect(extractMentions('@alice.')).toEqual(['alice']);
  });

  it('does not extract mentions across emphasis boundaries', () => {
    expect(extractMentions('@al*ice*')).toEqual(['al']);
    expect(extractMentions('@*alice*')).toEqual([]);
  });

  it('ignores mentions inside inline code', () => {
    expect(extractMentions('`@alice` @bob')).toEqual(['bob']);
  });

  it('ignores mentions inside escaped-backtick inline code', () => {
    expect(extractMentions('\\`@alice\\` @bob')).toEqual(['bob']);
  });

  it('extracts mentions immediately after inline code', () => {
    expect(extractMentions('`cmd`@alice')).toEqual(['alice']);
    expect(extractMentions('see`cmd`@alice')).toEqual(['alice']);
    expect(extractMentions('see `cmd`@alice')).toEqual(['alice']);
  });

  it('extracts mentions immediately after escaped-backtick inline code', () => {
    expect(extractMentions('\\`cmd\\`@alice')).toEqual(['alice']);
  });

  it('ignores mentions inside fenced code blocks', () => {
    expect(extractMentions('```\n@all\n```\n@bob')).toEqual(['bob']);
  });

  it('ignores mentions inside indented code blocks', () => {
    expect(extractMentions('    @alice\n@bob')).toEqual(['bob']);
  });

  it('ignores mentions inside blockquotes', () => {
    expect(extractMentions('> @alice said hi\n\n@bob replied')).toEqual(['bob']);
  });

  it('preserves mention order around excluded markdown regions', () => {
    expect(extractMentions('@alice `@bob` @charlie\n> @dora\n```\n@erin\n```\n@frank')).toEqual([
      'alice',
      'charlie',
      'frank'
    ]);
  });

  it('does not treat unmatched backticks as code spans', () => {
    expect(extractMentions('` @alice')).toEqual(['alice']);
  });

  it('treats literal html code tags as plain markdown text', () => {
    expect(extractMentions('<code>@alice</code>')).toEqual(['alice']);
  });

  it('keeps a backslash before a mention as a mention boundary', () => {
    expect(extractMentions('\\@alice')).toEqual(['alice']);
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
