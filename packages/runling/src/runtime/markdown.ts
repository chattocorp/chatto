import { ansiColor } from "./ansi.ts";
import { Markdown, type MarkdownTheme } from "@earendil-works/pi-tui";

const foreground = (color: string) => (text: string) => {
  const open = ansiColor(color);
  return open ? `${open}${text}\x1b[39m` : text;
};

const style = (open: number, close: number) => (text: string) =>
  `\x1b[${open}m${text}\x1b[${close}m`;

const bold = style(1, 22);
const dim = style(2, 22);

const theme: MarkdownTheme = {
  heading: (text) => bold(foreground("white")(text)),
  link: foreground("cyan"),
  linkUrl: dim,
  code: foreground("magenta"),
  codeBlock: foreground("white"),
  codeBlockBorder: dim,
  quote: dim,
  quoteBorder: foreground("gray"),
  hr: foreground("gray"),
  listBullet: foreground("cyan"),
  bold,
  italic: style(3, 23),
  strikethrough: style(9, 29),
  underline: style(4, 24),
};

/** Renders Markdown as width-aware ANSI terminal text. */
export const renderMarkdown = (markdown: string, width: number): string =>
  new Markdown(markdown, 0, 0, theme)
    .render(
      Number.isFinite(width) && width > 0
        ? Math.max(1, Math.floor(width))
        : 80,
    )
    .map((line) => line.trimEnd())
    .join("\n");
