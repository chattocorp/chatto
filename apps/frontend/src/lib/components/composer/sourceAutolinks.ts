/** Keep source-editor URL highlighting aligned with automatic link rendering. */
import { Autolink, type MarkdownConfig } from '@lezer/markdown';
import { hasAutoLinkPrefix } from '$lib/markdownLinks';

/** Retain Lezer's URL grammar and apply the message-link prefix policy. */
export const sourceAutolinks: MarkdownConfig = {
  ...Autolink,
  parseInline: Autolink.parseInline?.map((parser) => ({
    ...parser,
    parse(context, next, position) {
      const offset = position - context.offset;
      if (
        context.text[offset - 1] === ':' &&
        /[a-z][a-z0-9+.-]*:$/i.test(context.text.slice(0, offset))
      )
        return -1;
      if (!hasAutoLinkPrefix(context.text.slice(offset))) return -1;
      return parser.parse(context, next, position);
    }
  }))
};
