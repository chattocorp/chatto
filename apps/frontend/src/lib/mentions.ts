import MarkdownIt from 'markdown-it';
import type { RoomMember } from '$lib/state/room';
import type StateInline from 'markdown-it/lib/rules_inline/state_inline.mjs';

// Re-export for convenience
export type { RoomMember };

/**
 * Creates a fresh mention regex. We use a factory function instead of a module-level
 * regex with the `g` flag to avoid issues with persistent lastIndex state.
 *
 * Pattern matches @username where username contains alphanumeric, underscores, hyphens, and dots.
 * Dots are only allowed as internal separators (not trailing).
 * Used for wrapping mentions in already-rendered DOM text nodes.
 */
function createMentionRegex(): RegExp {
  return /(^|[^a-zA-Z0-9])@([a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)*)/g;
}

function isMentionAlphanumeric(value: string): boolean {
  return /^[a-zA-Z0-9]$/.test(value);
}

function isMentionHandleChar(value: string): boolean {
  return /^[a-zA-Z0-9_-]$/.test(value);
}

function mentionRule(state: StateInline, silent: boolean): boolean {
  const start = state.pos;
  if (state.src[start] !== '@') return false;
  if (start > 0 && isMentionAlphanumeric(state.src[start - 1])) return false;

  let stop = start + 1;
  while (stop < state.posMax && isMentionHandleChar(state.src[stop])) {
    stop++;
  }
  if (stop === start + 1) return false;

  while (stop < state.posMax && state.src[stop] === '.') {
    const next = stop + 1;
    if (next >= state.posMax || !isMentionHandleChar(state.src[next])) break;
    stop = next + 1;
    while (stop < state.posMax && isMentionHandleChar(state.src[stop])) {
      stop++;
    }
  }

  if (!silent) {
    const token = state.push('mention', '', 0);
    token.content = state.src.slice(start + 1, stop);
    token.markup = '@';
  }
  state.pos = stop;
  return true;
}

const mentionMarkdown = new MarkdownIt({
  html: false,
  linkify: false,
  breaks: true
});
mentionMarkdown.disable(['escape']);
mentionMarkdown.inline.ruler.before('emphasis', 'mention', mentionRule);

/**
 * Extract @usernames from text (without validation).
 * Returns deduplicated list of usernames in order of appearance.
 * Ignores mentions inside Markdown code spans, code blocks, and blockquotes.
 */
export function extractMentions(text: string): string[] {
  if (!text.includes('@')) return [];

  const mentions: string[] = [];

  let blockquoteDepth = 0;
  for (const token of mentionMarkdown.parse(text, {})) {
    if (token.type === 'blockquote_open') {
      blockquoteDepth++;
      continue;
    }
    if (token.type === 'blockquote_close') {
      blockquoteDepth = Math.max(0, blockquoteDepth - 1);
      continue;
    }
    if (blockquoteDepth > 0) continue;
    if (token.type === 'fence' || token.type === 'code_block') continue;
    if (token.type === 'inline') {
      for (const child of token.children ?? []) {
        if (child.type === 'mention') {
          mentions.push(child.content);
        }
      }
    }
  }

  return [...new Set(mentions)]; // Deduplicate
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
 * handle. Uses extractMentions, so Markdown code and blockquote regions are
 * ignored consistently with server-side mention resolution.
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
 * Elements whose text content should NOT have mention styling applied.
 * These are typically code/preformatted content or quoted material.
 */
const EXCLUDED_ELEMENTS = ['PRE', 'CODE', 'BLOCKQUOTE'];

/**
 * Check if a node is inside an excluded element (code, pre, blockquote).
 */
function isInsideExcludedElement(node: Node): boolean {
  let current: Node | null = node.parentNode;
  while (current && current.nodeType === Node.ELEMENT_NODE) {
    if (EXCLUDED_ELEMENTS.includes((current as Element).tagName)) {
      return true;
    }
    current = current.parentNode;
  }
  return false;
}

/**
 * Wrap valid @mentions in rendered HTML with styling.
 *
 * Uses DOMParser to properly traverse the DOM tree, ensuring we only process
 * text nodes that are NOT inside excluded elements (code, pre, blockquote).
 * Only mentions that match actual room members, virtual handles, or known
 * role handles are styled.
 *
 * @param html - The rendered HTML string (from markdown)
 * @param members - List of room members to validate mentions against
 * @param currentUserLogin - Optional login of the current user (for self-mention highlighting)
 * @param roleHandles - Valid role mention handles
 * @returns HTML string with valid mentions wrapped in <span class="mention"> (or "mention mention-self")
 */
export function wrapValidMentions(
  html: string,
  members: RoomMember[],
  currentUserLogin?: string,
  roleHandles: string[] = []
): string {
  // Handle empty input
  if (!html) {
    return html;
  }

  // Quick skip if no @ symbol (avoid DOM parsing)
  if (!html.includes('@')) {
    return html;
  }

  // Parse HTML into a DOM tree
  const parser = new DOMParser();
  const doc = parser.parseFromString(html, 'text/html');

  // Collect all text nodes that need processing
  const textNodes: Text[] = [];
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT);

  let node: Text | null;
  while ((node = walker.nextNode() as Text | null)) {
    // Skip text nodes inside excluded elements
    if (!isInsideExcludedElement(node)) {
      textNodes.push(node);
    }
  }

  // Process each text node
  const regex = createMentionRegex();
  for (const textNode of textNodes) {
    const text = textNode.textContent || '';
    if (!text.includes('@')) continue; // Quick skip if no @ symbol

    // Find all mentions in this text node
    const fragments: (string | Element)[] = [];
    let lastIndex = 0;
    let match;

    while ((match = regex.exec(text)) !== null) {
      const [fullMatch, prefix, username] = match;
      const matchStart = match.index;

      // Add text before this match
      if (matchStart > lastIndex) {
        fragments.push(text.slice(lastIndex, matchStart));
      }

      // Add the prefix (character before @, if any)
      if (prefix) {
        fragments.push(prefix);
      }

      // Check if this is a valid mention (matches a room member or virtual handle)
      const mentionedMember = findMemberByMention(username, members);
      if (mentionedMember) {
        // Create styled element for valid mention
        const span = doc.createElement('span');
        const isSelfMention =
          currentUserLogin &&
          mentionedMember.login.toLowerCase() === currentUserLogin.toLowerCase();
        span.className = isSelfMention ? 'mention mention-self' : 'mention';
        span.setAttribute('data-user-id', mentionedMember.id);
        span.textContent = `@${username}`;
        fragments.push(span);
      } else if (isVirtualMention(username)) {
        const span = doc.createElement('span');
        span.className = 'mention mention-broadcast';
        span.textContent = `@${username}`;
        fragments.push(span);
      } else {
        const roleName = findRoleMention(username, roleHandles);
        if (roleName) {
          const span = doc.createElement('span');
          span.className = 'mention mention-role';
          span.setAttribute('data-role-name', roleName);
          span.textContent = `@${username}`;
          fragments.push(span);
        } else {
          // Leave invalid mentions as plain text
          fragments.push(`@${username}`);
        }
      }

      lastIndex = matchStart + fullMatch.length;
    }

    // Add remaining text after last match
    if (lastIndex < text.length) {
      fragments.push(text.slice(lastIndex));
    }

    // Only replace if we found any mentions
    if (lastIndex > 0) {
      // Replace the text node with our fragments
      const parent = textNode.parentNode;
      if (parent) {
        for (const fragment of fragments) {
          if (typeof fragment === 'string') {
            parent.insertBefore(doc.createTextNode(fragment), textNode);
          } else {
            parent.insertBefore(fragment, textNode);
          }
        }
        parent.removeChild(textNode);
      }
    }
  }

  return doc.body.innerHTML;
}
