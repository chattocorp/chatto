/** Lexical Bend 2 highlighting for message fences and both composers.
 * Highlight.js has no Bend grammar. This one follows the syntax reference in
 * https://github.com/bendlang/bend/blob/main/guide/GUIDE.md and decorates
 * source only: it does not parse or check programs.
 */
import type { LanguageFn } from 'highlight.js';

const IDENT = /[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*/;

const bend: LanguageFn = (hljs) => ({
  name: 'Bend',
  keywords: {
    keyword: 'import as type is def law for where exs match case do return',
    type: [
      'Type',
      'Data',
      'Kind',
      'Quant',
      'Nat',
      'U32',
      'F32',
      'Char',
      'String',
      'Bool',
      'Unit',
      'Empty',
      'Cmp',
      'Maybe',
      'Result',
      'List',
      'Array',
      'Map',
      'Equal',
      'IO',
      'Chan'
    ],
    literal: 'True False Nil Con Some None'
  },
  contains: [
    hljs.HASH_COMMENT_MODE,
    hljs.QUOTE_STRING_MODE,
    { scope: 'string', begin: /'/, end: /'/, illegal: /\n/, contains: [hljs.BACKSLASH_ESCAPE] },
    // Definition heads: `def name`, `law name`, `type Name`.
    {
      match: [/\b(?:def|law)/, /\s+/, IDENT],
      scope: { 1: 'keyword', 3: 'title.function' }
    },
    {
      match: [/\btype/, /\s+/, IDENT],
      scope: { 1: 'keyword', 3: 'title.class' }
    },
    // `@unsafe` marks a definition; `?goal` prints or leaves a proof goal open.
    { scope: 'meta', match: /@unsafe\b/ },
    { scope: 'meta', match: /\?[A-Za-z_][A-Za-z0-9_]*/ },
    // `{==}` is the reflexivity proof term.
    { scope: 'literal', match: /\{==\}/ },
    // Nat (`3n`), F32 (`1.5`) and U32 (`42`) literals.
    { scope: 'number', match: /\b\d+(?:n|\.\d+)?\b/ },
    { scope: 'operator', match: /->|=>|<-|==|!=|<>|\+\+|&&|\|\||<<|>>|\.[&|^]\./ }
  ]
});

export default bend;
