/** Shared automatic-link rules for message rendering and preview detection. */
import type MarkdownIt from 'markdown-it';
import tlds from 'tlds';

/** Message links can navigate to web pages or hand an address to a mail app. */
const allowedLinkProtocols = new Map([
  ['http:', '//'],
  ['https:', '//'],
  ['mailto:', '']
]);

/** Check the explicit protocol against the message-link allow list. */
export function hasAllowedLinkProtocol(text: string): boolean {
  const protocol = text.match(/^[a-z][a-z0-9+.-]*:/i)?.[0];
  if (!protocol) return false;
  const suffix = allowedLinkProtocols.get(protocol.toLowerCase());
  return suffix !== undefined && text.slice(protocol.length).startsWith(suffix);
}

/** Require an allowed explicit protocol or www. before automatic link creation. */
export function hasAutoLinkPrefix(text: string): boolean {
  return /^www\./i.test(text) || hasAllowedLinkProtocol(text);
}

/** Preview fetches are limited to allowed web URLs; mail links never fetch. */
export function isPreviewURL(text: string): boolean {
  return hasAllowedLinkProtocol(text) && /^https?:\/\//i.test(text);
}

/** Keep filename-like text plain while retaining linkify-it's URL validation. */
export function configureMarkdownLinks(markdown: MarkdownIt): void {
  const linkify = markdown.linkify;
  linkify.tlds(tlds);
  linkify.set({ fuzzyLink: false, fuzzyEmail: false });
  linkify.add('//', null);
  linkify.add('ftp:', null);
  linkify.add('www.', {
    validate(text, position, self) {
      // Reuse the library's host, port, and path grammar, including known TLDs.
      const start = position - 4;
      const offset = Math.max(0, start - 1);
      const match = self.re.link_no_ip_fuzzy.exec(text.slice(offset));
      return match && offset + match.index + match[1].length === start ? match[2].length - 4 : 0;
    },
    normalize(match) {
      match.url = `https://${match.raw}`;
    }
  });

  // Retain explicit link tokens for syntax-aware mention extraction, but hide
  // unsupported link marks so their labels render without inert href="#" links.
  markdown.core.ruler.after('linkify', 'allowed_link_protocols', (state) => {
    for (const token of state.tokens) {
      if (!token.children) continue;
      const hidden: boolean[] = [];
      token.children.forEach((child, index, children) => {
        if (child.type === 'link_open') {
          const previous = children[index - 1];
          // Do not link the HTTPS part of an unsupported URI such as blob:https://.
          const nestedProtocol =
            child.markup === 'linkify' &&
            previous?.type === 'text' &&
            /[a-z][a-z0-9+.-]*:$/i.test(previous.content);
          const keep = !nestedProtocol && hasAllowedLinkProtocol(child.attrGet('href') ?? '');
          hidden.push(!keep);
          child.hidden = !keep;
        } else if (child.type === 'link_close') {
          child.hidden = hidden.pop() ?? false;
        }
      });
    }
  });
}
