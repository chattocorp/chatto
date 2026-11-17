/** The Odin grammar package exports a Highlight.js language without types. */
declare module 'highlightjs-odinlang' {
  import type { LanguageFn } from 'highlight.js';

  const odin: LanguageFn;
  export default odin;
}
