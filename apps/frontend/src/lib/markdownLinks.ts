/** Shared automatic-link rules for message rendering and preview detection. */
import type LinkifyIt from 'linkify-it';
import tlds from 'tlds';

/** Require an explicit scheme or www. prefix before automatic link creation. */
export function hasAutoLinkPrefix(text: string): boolean {
  return /^(?:www\.|[a-z][a-z0-9+.-]*:\/\/|mailto:)/i.test(text);
}

/** Keep filename-like text plain while retaining linkify-it's URL validation. */
export function configureMarkdownLinks(linkify: LinkifyIt): void {
  linkify.tlds(tlds);
  linkify.set({ fuzzyLink: false, fuzzyEmail: false });
  linkify.add('//', null);
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
}
