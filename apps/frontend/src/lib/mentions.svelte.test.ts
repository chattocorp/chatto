import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
/**
 * Browser tests for resolveRenderedMentions (requires DOMParser).
 * Pure function tests are in mentions.test.ts; mention tokenization tests are
 * in markdownMentions.test.ts.
 */
import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';
import { resolveRenderedMentions, type RoomMember } from './mentions';

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

/** Renders a message body and resolves its mentions, like MessageContent. */
async function render(
  body: string,
  members: RoomMember[],
  currentUserLogin?: string,
  roleHandles?: string[]
): Promise<string> {
  return resolveRenderedMentions(
    await renderMarkdown(body, { mentions: true }),
    members,
    currentUserLogin,
    roleHandles
  );
}

describe('resolveRenderedMentions', () => {
  const members = [member('alice', 'Alice'), member('bob', 'Bob')];
  const aliceMention = '<span class="mention" data-user-id="alice" dir="auto">@Alice</span>';
  const bobMention = '<span class="mention" data-user-id="bob" dir="auto">@Bob</span>';
  const selfMention =
    '<span class="mention mention-self" data-user-id="alice" dir="auto">@Alice</span>';

  it('resolves a member mention', async () => {
    expect(await render('Hello @alice!', members)).toBe(`<p>Hello ${aliceMention}!</p>\n`);
  });

  it('shows the current display name while keeping the original member target', async () => {
    const result = await render('Hello @hendrik!', [member('hendrik', 'Hendrik Mans')]);
    expect(result).toContain(
      '<span class="mention" data-user-id="hendrik" dir="auto">@Hendrik Mans</span>'
    );
    expect(result).not.toContain('@hendrik');
  });

  it('escapes display names and falls back to the login when the name is empty', async () => {
    const result = await render('@alice @bob', [
      member('alice', '<Alice & Bob>'),
      member('bob', '')
    ]);
    expect(result).toContain('@&lt;Alice &amp; Bob&gt;</span>');
    expect(result).toContain('<span class="mention" data-user-id="bob" dir="auto">@bob</span>');
  });

  it('turns unknown handles into plain text', async () => {
    expect(await render('Hello @charlie!', members)).toBe('<p>Hello @charlie!</p>\n');
  });

  it('resolves mixed valid and invalid mentions', async () => {
    expect(await render('@alice @charlie @bob', members)).toBe(
      `<p>${aliceMention} @charlie ${bobMention}</p>\n`
    );
  });

  it('resolves every occurrence of a repeated mention', async () => {
    const result = await render('@alice @bob @alice', members);
    expect(result.match(/<span class="mention"/g)).toHaveLength(3);
  });

  it('is case-insensitive for member matching', async () => {
    expect(await render('Hello @ALICE!', members)).toContain(aliceMention);
  });

  it('resolves mentions separated by a slash', async () => {
    expect(await render('@alice/@bob', members)).toContain(`${aliceMention}/${bobMention}`);
  });

  it('keeps a member handle in a URL as literal link text', async () => {
    const url = 'https://social.5f9.de/@alice/117331230238178837';
    const result = await render(`Zum Thema ${url}`, members);
    expect(result).toContain(`>${url}</a>`);
    expect(result).not.toContain('class="mention');
  });

  it('keeps a member handle in link text literal', async () => {
    const result = await render('[@alice](https://example.com) and @bob', members);
    expect(result).toContain('>@alice</a>');
    expect(result).not.toContain('data-user-id="alice"');
    expect(result).toContain(bobMention);
  });

  it('does not resolve mentions in code or blockquotes', async () => {
    const result = await render('@alice says `@bob`\n\n> @bob wrote', members);
    expect(result).toContain(aliceMention);
    expect(result).not.toContain('data-user-id="bob"');
  });

  it('resolves mentions in emphasis', async () => {
    expect(await render('Hey *@alice*', members)).toContain(`<em>${aliceMention}</em>`);
  });

  it('returns HTML without mention candidates unchanged', () => {
    const html = '<p>Hello world!</p>\n';
    expect(resolveRenderedMentions(html, members)).toBe(html);
    expect(resolveRenderedMentions('', members)).toBe('');
  });

  it('does not scan text for mentions', () => {
    const html = '<p>Hello @alice!</p>';
    expect(resolveRenderedMentions(html, members)).toBe(html);
  });

  describe('virtual and role mentions', () => {
    it('resolves virtual handles', async () => {
      expect(await render('@all @here', members)).toBe(
        '<p><span class="mention mention-broadcast">@all</span> <span class="mention mention-broadcast">@here</span></p>\n'
      );
    });

    it('resolves known role handles', async () => {
      const result = await render('@admin @owner @support @unknown', members, undefined, [
        'admin',
        'owner',
        'support'
      ]);

      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="admin">@admin</span>'
      );
      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="owner">@owner</span>'
      );
      expect(result).toContain(
        '<span class="mention mention-role" data-role-name="support">@support</span>'
      );
      expect(result).toContain(' @unknown</p>');
    });

    it('matches role handles case-insensitively', async () => {
      expect(await render('Hello @ADMIN', members, undefined, ['admin'])).toContain(
        '<span class="mention mention-role" data-role-name="admin">@ADMIN</span>'
      );
    });
  });

  describe('self-mention highlighting', () => {
    it('adds mention-self class when current user is mentioned', async () => {
      expect(await render('Hello @alice!', members, 'alice')).toContain(selfMention);
    });

    it('does not add mention-self class for other users', async () => {
      const result = await render('Hello @bob!', members, 'alice');
      expect(result).toContain(bobMention);
      expect(result).not.toContain('mention-self');
    });

    it('is case-insensitive for current user matching', async () => {
      expect(await render('Hello @ALICE!', members, 'alice')).toContain(selfMention);
    });

    it('works without currentUserLogin parameter', async () => {
      const result = await render('Hello @alice!', members);
      expect(result).toContain(aliceMention);
      expect(result).not.toContain('mention-self');
    });
  });
});
