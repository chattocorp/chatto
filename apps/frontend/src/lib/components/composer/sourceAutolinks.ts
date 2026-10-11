/** Keep source-editor URL highlighting aligned with automatic link rendering. */
import { Autolink, type MarkdownConfig } from '@lezer/markdown';
import { hasAutoLinkPrefix } from '$lib/markdownLinks';

/** Retain Lezer's URL grammar but exclude email addresses without mailto:. */
export const sourceAutolinks: MarkdownConfig = {
  ...Autolink,
  parseInline: Autolink.parseInline?.map((parser) => ({
    ...parser,
    parse(context, next, position) {
      if (!hasAutoLinkPrefix(context.text.slice(position - context.offset))) return -1;
      return parser.parse(context, next, position);
    }
  }))
};
