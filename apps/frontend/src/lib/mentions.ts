import type { RoomMember } from '$lib/state/room';
import { extractMarkdownMentions } from '$lib/markdown';
import { MENTION_HANDLE_ATTRIBUTE } from '$lib/markdownMentions';
import { parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';

// Re-export for convenience
export type { RoomMember };

/**
 * Extract @usernames from text (without validation).
 * Returns deduplicated list of usernames in order of appearance.
 * Uses the message renderer's parser, so mentions inside Markdown code,
 * blockquotes, links, and linkified URLs are ignored exactly as in the
 * rendered message.
 */
export function extractMentions(text: string): string[] {
  return extractMarkdownMentions(text);
}

/**
 * Check if a username matches a room member (case-insensitive).
 * Matches against both login and displayName.
 */
export function findMemberByMention(
  username: string,
  members: RoomMember[]
): RoomMember | undefined {
  const lower = username.toLowerCase();
  return members.find(
    (m) => m.login.toLowerCase() === lower || m.displayName.toLowerCase() === lower
  );
}

function isVirtualMention(username: string): boolean {
  const lower = username.toLowerCase();
  return lower === 'all' || lower === 'here';
}

/**
 * Reports whether text mentions a room-wide virtual group or any known role
 * handle. Uses extractMentions, so Markdown code, blockquote, and link regions
 * are ignored consistently with server-side mention resolution.
 */
export function hasRoleOrVirtualMention(text: string, roleHandles: string[]): boolean {
  const roles = new Set(roleHandles.map((role) => role.toLowerCase()));
  return extractMentions(text).some(
    (mention) => isVirtualMention(mention) || roles.has(mention.toLowerCase())
  );
}

function findRoleMention(username: string, roleHandles: string[]): string | undefined {
  const lower = username.toLowerCase();
  return roleHandles.find((role) => role.toLowerCase() === lower);
}

/**
 * Check if a specific user is mentioned in text.
 * Uses the room members list to validate that mentions refer to actual users.
 */
export function isUserMentioned(text: string, userLogin: string, members: RoomMember[]): boolean {
  const mentions = extractMentions(text);
  const lower = userLogin.toLowerCase();
  return mentions.some((mention) => {
    const member = findMemberByMention(mention, members);
    return member?.login.toLowerCase() === lower;
  });
}

/**
 * Builds the element for one mention candidate, or returns null when the
 * handle matches no room member, virtual handle, or known role handle.
 */
function resolveMention(
  doc: Document,
  handle: string,
  members: RoomMember[],
  currentUserLogin: string | undefined,
  roleHandles: string[]
): HTMLSpanElement | null {
  const span = doc.createElement('span');

  const member = findMemberByMention(handle, members);
  if (member) {
    const isSelfMention =
      currentUserLogin && member.login.toLowerCase() === currentUserLogin.toLowerCase();
    span.className = isSelfMention ? 'mention mention-self' : 'mention';
    span.setAttribute('data-user-id', member.id);
    span.setAttribute('dir', 'auto');
    span.textContent = `@${member.displayName.trim() || member.login}`;
    return span;
  }

  if (isVirtualMention(handle)) {
    span.className = 'mention mention-broadcast';
    span.textContent = `@${handle}`;
    return span;
  }

  const roleName = findRoleMention(handle, roleHandles);
  if (roleName) {
    span.className = 'mention mention-role';
    span.setAttribute('data-role-name', roleName);
    span.textContent = `@${handle}`;
    return span;
  }

  return null;
}

/**
 * Resolve the mention candidates in HTML from `renderMarkdown`.
 *
 * The markdown renderer marks each `@handle` that it recognizes in plain
 * message text with a `data-mention-handle` span; see `$lib/markdownMentions`.
 * This step only replaces those marked elements and never scans text, so
 * links, URLs, code, and blockquotes keep their literal text. Candidates that
 * match room members, virtual handles, or known role handles become styled
 * mentions. User mentions show the member's current display name. Other
 * candidates become plain `@handle` text.
 *
 * @param html - HTML from `renderMarkdown`
 * @param members - List of room members to validate mentions against
 * @param currentUserLogin - Optional login of the current user (for self-mention highlighting)
 * @param roleHandles - Valid role mention handles
 * @returns HTML string with valid mentions as `<span class="mention">` (or "mention mention-self")
 */
export function resolveRenderedMentions(
  html: string,
  members: RoomMember[],
  currentUserLogin?: string,
  roleHandles: string[] = []
): string {
  // Quick skip without DOM parsing when the renderer emitted no candidates.
  if (!html.includes(MENTION_HANDLE_ATTRIBUTE)) return html;

  const doc = parseTrustedMarkdownHtml(html);
  for (const candidate of doc.body.querySelectorAll(`span[${MENTION_HANDLE_ATTRIBUTE}]`)) {
    const handle = candidate.getAttribute(MENTION_HANDLE_ATTRIBUTE) ?? '';
    const mention = resolveMention(doc, handle, members, currentUserLogin, roleHandles);
    candidate.replaceWith(mention ?? doc.createTextNode(`@${handle}`));
  }

  return doc.body.innerHTML;
}
