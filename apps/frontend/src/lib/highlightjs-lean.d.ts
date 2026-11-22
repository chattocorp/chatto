/** The Lean grammar package exports a Highlight.js language without types. */
declare module 'highlightjs-lean' {
  import type { LanguageFn } from 'highlight.js';

  const lean: LanguageFn;
  export default lean;
}
