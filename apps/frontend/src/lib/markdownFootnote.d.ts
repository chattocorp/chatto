// The upstream declarations import markdown-it's incompatible CommonJS types.
// Type the plugin's ESM entry with the same MarkdownIt type as the renderer.
declare module 'markdown-it-footnote' {
  import type MarkdownIt from 'markdown-it';

  /** Registers the footnote parser and renderer rules on this instance. */
  export default function footnotePlugin(md: MarkdownIt): void;
}
