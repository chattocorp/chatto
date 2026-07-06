/**
 * Browser tests for wrapValidMentions (requires DOMParser).
 * Pure function tests are in mentions.test.ts.
 */
import { describe, it, expect } from 'vitest';
import { wrapValidMentions, type RoomMember } from './mentions';
import type { PresenceStatus } from '$lib/render/types';

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

describe('wrapValidMentions', () => {
  const members = [member('alice', 'Alice'), member('bob', 'Bob')];

  it('wraps valid mention in span tag', () => {
    const result = wrapValidMentions('<p>Hello @alice!</p>', members);
    expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
  });

  it('does not wrap invalid mentions', () => {
    const result = wrapValidMentions('<p>Hello @charlie!</p>', members);
    expect(result).not.toContain('<span class="mention"');
    expect(result).toContain('@charlie');
  });

  it('wraps multiple valid mentions', () => {
    const result = wrapValidMentions('<p>Hey @alice and @bob</p>', members);
    expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    expect(result).toContain('<span class="mention" data-user-id="bob">@bob</span>');
  });

  it('handles mixed valid and invalid mentions', () => {
    const result = wrapValidMentions('<p>@alice @charlie @bob</p>', members);
    expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    expect(result).toContain('<span class="mention" data-user-id="bob">@bob</span>');
    expect(result).toContain('@charlie');
    expect((result.match(/<span class="mention"/g) || []).length).toBe(2);
  });

  describe('code block exclusion', () => {
    it('does not style mentions inside inline code', () => {
      const result = wrapValidMentions('<p>Text <code>@alice</code></p>', members);
      expect(result).toContain('<code>@alice</code>');
      expect(result).not.toContain('<code><span');
    });

    it('does not style mentions inside pre blocks', () => {
      const result = wrapValidMentions('<pre>@alice in code</pre>', members);
      expect(result).toContain('@alice in code');
      expect(result).not.toContain('<span class="mention">');
    });

    it('does not style mentions inside nested pre/code blocks', () => {
      const result = wrapValidMentions('<pre><code>@alice</code></pre>', members);
      expect(result).not.toContain('<span class="mention">');
    });

    it('styles mentions outside code but not inside', () => {
      const result = wrapValidMentions('<p>@alice says <code>@bob</code></p>', members);
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
      // bob inside code should not be styled
      expect(result).not.toContain('data-user-id="bob"');
    });

    it('styles mentions immediately after inline code', () => {
      const result = wrapValidMentions('<p><code>cmd</code>@alice</p>', members);
      expect(result).toContain('<code>cmd</code>');
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    });
  });

  describe('blockquote exclusion', () => {
    it('does not style mentions inside blockquotes', () => {
      const result = wrapValidMentions('<blockquote>@alice said</blockquote>', members);
      expect(result).toContain('@alice said');
      expect(result).not.toContain('<span class="mention">');
    });

    it('handles deeply nested blockquotes', () => {
      const result = wrapValidMentions('<blockquote><p><em>@alice</em></p></blockquote>', members);
      expect(result).not.toContain('<span class="mention">');
    });

    it('styles mentions outside blockquote but not inside', () => {
      const result = wrapValidMentions(
        '<p>@alice wrote:</p><blockquote>@bob is great</blockquote>',
        members
      );
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
      // bob inside blockquote should not be styled
      expect(result).not.toContain('data-user-id="bob"');
    });
  });

  describe('edge cases', () => {
    it('returns empty string unchanged', () => {
      expect(wrapValidMentions('', members)).toBe('');
    });

    it('returns html unchanged when no members', () => {
      const html = '<p>Hello @alice!</p>';
      expect(wrapValidMentions(html, [])).toBe(html);
    });

    it('returns html unchanged when no @ symbols', () => {
      const html = '<p>Hello world!</p>';
      // DOMParser may normalize HTML slightly, so just check content is preserved
      const result = wrapValidMentions(html, members);
      expect(result).toContain('Hello world!');
      expect(result).not.toContain('<span class="mention"');
    });

    it('preserves surrounding text and HTML structure', () => {
      const result = wrapValidMentions('<p>Hey <em>@alice</em> how are you?</p>', members);
      expect(result).toContain('Hey');
      expect(result).toContain('how are you?');
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    });

    it('handles mention at start of paragraph', () => {
      const result = wrapValidMentions('<p>@alice hello</p>', members);
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    });

    it('handles mention after punctuation', () => {
      const result = wrapValidMentions('<p>Hi, @alice!</p>', members);
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    });

    it('is case-insensitive for member matching', () => {
      const result = wrapValidMentions('<p>Hello @ALICE!</p>', members);
      expect(result).toContain('<span class="mention" data-user-id="alice">@ALICE</span>');
    });

    it('handles multiple mentions in same text node', () => {
      const result = wrapValidMentions('<p>@alice @bob @alice</p>', members);
      expect((result.match(/<span class="mention"/g) || []).length).toBe(3);
    });

    it('preserves other HTML elements', () => {
      const result = wrapValidMentions('<p>Check <a href="#">this link</a> @alice</p>', members);
      expect(result).toContain('<a href="#">this link</a>');
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
    });

    it('wraps known role mention handles', () => {
      const result = wrapValidMentions(
        '<p>@admin @owner @support @unknown</p>',
        members,
        undefined,
        ['admin', 'owner', 'support']
      );

      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="admin">@admin</span>'
      );
      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="owner">@owner</span>'
      );
      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="support">@support</span>'
      );
      expect(result).toContain('@unknown');
    });

    it('matches role mention handles case-insensitively', () => {
      const result = wrapValidMentions('<p>Hello @ADMIN</p>', members, undefined, ['admin']);
      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="admin">@ADMIN</span>'
      );
    });
  });

  describe('self-mention highlighting', () => {
    it('adds mention-self class when current user is mentioned', () => {
      const result = wrapValidMentions('<p>Hello @alice!</p>', members, 'alice');
      expect(result).toContain(
        '<span class="mention mention-self" data-user-id="alice">@alice</span>'
      );
    });

    it('does not add mention-self class for other users', () => {
      const result = wrapValidMentions('<p>Hello @bob!</p>', members, 'alice');
      expect(result).toContain('<span class="mention" data-user-id="bob">@bob</span>');
      expect(result).not.toContain('mention-self');
    });

    it('handles mixed self and other mentions', () => {
      const result = wrapValidMentions('<p>@alice and @bob</p>', members, 'alice');
      expect(result).toContain(
        '<span class="mention mention-self" data-user-id="alice">@alice</span>'
      );
      expect(result).toContain('<span class="mention" data-user-id="bob">@bob</span>');
    });

    it('is case-insensitive for current user matching', () => {
      const result = wrapValidMentions('<p>Hello @ALICE!</p>', members, 'alice');
      expect(result).toContain(
        '<span class="mention mention-self" data-user-id="alice">@ALICE</span>'
      );
    });

    it('works without currentUserLogin parameter', () => {
      const result = wrapValidMentions('<p>Hello @alice!</p>', members);
      expect(result).toContain('<span class="mention" data-user-id="alice">@alice</span>');
      expect(result).not.toContain('mention-self');
    });
  });
});
