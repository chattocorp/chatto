/** Visual footnote nodes retain named Markdown labels and editable block content. */
import { InputRule, Node, type JSONContent } from '@tiptap/core';
import type { Node as ProseMirrorNode } from '@tiptap/pm/model';
import { closeHistory } from '@tiptap/pm/history';
import { Plugin, TextSelection, type Transaction } from '@tiptap/pm/state';
import MarkdownIt from 'markdown-it';
import footnotePlugin from 'markdown-it-footnote';
import { m } from '$lib/i18n/messages';

const definitionParser = new MarkdownIt({ html: false }).use(footnotePlugin);
definitionParser.disable(['footnote_inline', 'footnote_tail']);
const definitionStart = /^ {0,3}\[\^([^\]\s]+)\]:[ \t]?/;
const referenceStart = /^\[\^([^\]\s]+)\]/;

/** Find definitions with the same code and indentation rules as rendered messages. */
export function definedFootnoteLabels(source: string): Set<string> {
  if (!source.includes('[^')) return new Set();
  const env: { footnotes?: { refs?: Record<string, number> } } = {};
  definitionParser.parse(source, env);
  return new Set(Object.keys(env.footnotes?.refs ?? {}).map((label) => label.slice(1)));
}

/** Use the display parser's block boundaries, including lazy continuation lines. */
function tokenizeDefinition(source: string) {
  const match = source.match(definitionStart);
  if (!match) return;
  const tokens = definitionParser.parse(source, {});
  if (tokens[0]?.type !== 'footnote_reference_open') return;
  let depth = 0;
  let endLine = 1;
  for (const token of tokens) {
    if (token.type === 'footnote_reference_open') depth++;
    if (token.map) endLine = Math.max(endLine, token.map[1]);
    if (token.type === 'footnote_reference_close' && --depth === 0) break;
  }
  const lines = source.split('\n');
  const raw = lines.slice(0, endLine).join('\n') + (endLine < lines.length ? '\n' : '');
  const text = raw.slice(match[0].length).replace(/\n(?: {4}|\t)/g, '\n');
  return { type: 'footnoteDefinition', raw, label: match[1], text };
}

/** Resolve only defined references and collect definitions at the end of the document. */
export function normalizeFootnotesForEditor(document: JSONContent): JSONContent {
  const definitions = new Map<string, JSONContent>();
  let hasReferences = false;
  const collect = (node: JSONContent) => {
    if (node.type === 'footnoteDefinition') definitions.set(node.attrs?.label, node);
    if (node.type === 'footnoteReference') hasReferences = true;
    node.content?.forEach(collect);
  };
  collect(document);
  if (definitions.size === 0 && !hasReferences) return document;
  const numbers = new Map<string, number>();
  const resolve = (node: JSONContent): JSONContent[] => {
    if (node.type === 'footnoteDefinition') return [];
    if (node.type === 'footnoteReference') {
      const label: string = node.attrs?.label;
      if (!definitions.has(label))
        return [
          { type: 'text', text: `[^${label}]`, ...(node.marks ? { marks: node.marks } : {}) }
        ];
      if (!numbers.has(label)) numbers.set(label, numbers.size + 1);
      return [{ ...node, attrs: { ...node.attrs, number: numbers.get(label) } }];
    }
    if (!node.content) return [node];
    const content = node.content.flatMap(resolve);
    if (
      content.length === 0 &&
      node.content.length > 0 &&
      node.type !== 'paragraph' &&
      node.type !== 'heading'
    )
      return [];
    return [{ ...node, content }];
  };
  const body = document.content?.flatMap(resolve) ?? [];
  // Referenced notes use reference order. Unused definitions remain editable.
  const labels = [
    ...numbers.keys(),
    ...[...definitions.keys()].filter((label) => !numbers.has(label))
  ];
  const notes = labels.map((label, index) => {
    const note = definitions.get(label)!;
    const content = note.content?.flatMap(resolve) ?? [];
    return {
      ...note,
      attrs: { ...note.attrs, number: index + 1 },
      content: content.length ? content : [{ type: 'paragraph' }]
    };
  });
  return { ...document, content: [...body, ...notes] };
}

function footnoteNumbers(document: ProseMirrorNode): Map<string, number> {
  const numbers = new Map<string, number>();
  document.descendants((node) => {
    if (node.type.name === 'footnoteDefinition') return false;
    if (node.type.name === 'footnoteReference' && !numbers.has(node.attrs.label)) {
      numbers.set(node.attrs.label, numbers.size + 1);
    }
  });
  document.descendants((node) => {
    if (node.type.name === 'footnoteDefinition' && !numbers.has(node.attrs.label)) {
      numbers.set(node.attrs.label, numbers.size + 1);
    }
  });
  return numbers;
}

/** Select the first text block, including inside a list or quote, within this note. */
function selectFootnoteText(tr: Transaction, notePosition: number): void {
  const note = tr.doc.nodeAt(notePosition)!;
  let textPosition: number | undefined;
  note.descendants((node, offset) => {
    if (textPosition !== undefined) return false;
    if (node.isTextblock) {
      textPosition = notePosition + offset + 2;
      return false;
    }
  });
  if (textPosition === undefined) {
    // A note made only of block atoms needs a paragraph for text entry.
    const end = notePosition + note.nodeSize - 1;
    tr.insert(end, tr.doc.type.schema.nodes.paragraph.create());
    textPosition = end + 1;
  }
  tr.setSelection(TextSelection.create(tr.doc, textPosition));
  tr.scrollIntoView();
}

/** Insert a marker after the selection and focus its existing or new note. */
export function insertVisualFootnote(tr: Transaction, requestedLabel?: string): boolean {
  if (
    tr.selection.$from.parent.type.spec.code ||
    (tr.storedMarks ?? tr.selection.$from.marks()).some((mark) => mark.type.spec.code)
  )
    return false;
  const { schema } = tr.doc.type;
  const labels = footnoteNumbers(tr.doc);
  let next = 1;
  while (labels.has(String(next))) next++;
  const label = requestedLabel ?? String(next);
  // The marker and its definition form one undo step, separate from preceding text.
  closeHistory(tr);
  tr.setSelection(TextSelection.create(tr.doc, tr.selection.to));
  tr.replaceSelectionWith(schema.nodes.footnoteReference.create({ label }), false);
  let notePosition: number | undefined;
  tr.doc.descendants((node, position) => {
    if (node.type.name === 'footnoteDefinition' && node.attrs.label === label)
      notePosition = position;
  });
  if (notePosition === undefined) {
    const numbers = footnoteNumbers(tr.doc);
    const number = numbers.get(label)!;
    notePosition = tr.doc.content.size;
    tr.doc.forEach((node, position) => {
      if (node.type.name === 'footnoteDefinition' && numbers.get(node.attrs.label)! > number) {
        notePosition = Math.min(notePosition!, position);
      }
    });
    tr.insert(
      notePosition,
      schema.nodes.footnoteDefinition.create({ label, number }, schema.nodes.paragraph.create())
    );
  }
  selectFootnoteText(tr, notePosition);
  return true;
}

/** An inline atom links repeated markers to the same editable definition. */
export const FootnoteReference = Node.create({
  name: 'footnoteReference',
  inline: true,
  group: 'inline',
  atom: true,

  addAttributes() {
    return {
      label: { default: '', rendered: false },
      number: { default: 1, rendered: false }
    };
  },

  parseHTML() {
    return [
      {
        tag: 'sup[data-footnote-label]',
        getAttrs: (node) => ({ label: node.dataset.footnoteLabel })
      }
    ];
  },

  renderHTML({ node }) {
    return [
      'sup',
      {
        class: 'composer-footnote-ref',
        'data-footnote-label': node.attrs.label,
        role: 'button',
        tabindex: '0',
        'aria-label': m('composer.footnote_reference', { number: node.attrs.number })
      },
      `[${node.attrs.number}]`
    ];
  },

  renderText({ node }) {
    return `[^${node.attrs.label}]`;
  },

  markdownTokenizer: {
    name: 'footnoteReference',
    level: 'inline',
    start: (source) => source.indexOf('[^'),
    tokenize(source) {
      const match = source.match(referenceStart);
      if (match) return { type: 'footnoteReference', raw: match[0], label: match[1] };
    }
  },

  parseMarkdown: (token) => ({ type: 'footnoteReference', attrs: { label: token.label } }),
  renderMarkdown: (node) => `[^${node.attrs?.label}]`,

  addInputRules() {
    return [
      new InputRule({
        find: /\[\^([^\]\s]+)\]$/,
        handler: ({ state, range, match }) => {
          if (
            state.selection.$from.parent.type.spec.code ||
            state.selection.$from.marks().some((mark) => mark.type.spec.code)
          )
            return null;
          insertVisualFootnote(state.tr.delete(range.from, range.to), match[1]);
        }
      })
    ];
  },

  addProseMirrorPlugins() {
    const focusNote = (label: string) => {
      let position: number | undefined;
      this.editor.state.doc.descendants((node, pos) => {
        if (node.type.name === 'footnoteDefinition' && node.attrs.label === label) position = pos;
      });
      if (position === undefined) return false;
      const notePosition = position;
      return this.editor
        .chain()
        .command(({ tr }) => {
          selectFootnoteText(tr, notePosition);
          return true;
        })
        .focus()
        .run();
    };
    return [
      new Plugin({
        appendTransaction(transactions, _oldState, state) {
          if (!transactions.some((tr) => tr.docChanged)) return null;
          const numbers = footnoteNumbers(state.doc);
          const tr = state.tr;
          state.doc.descendants((node, position) => {
            if (node.type.name !== 'footnoteReference' && node.type.name !== 'footnoteDefinition')
              return;
            const number = numbers.get(node.attrs.label);
            if (number !== node.attrs.number)
              tr.setNodeMarkup(position, undefined, { ...node.attrs, number });
          });
          if (!tr.docChanged) return null;
          tr.setMeta('addToHistory', false);
          if (transactions.some((transaction) => transaction.getMeta('preventUpdate')))
            tr.setMeta('preventUpdate', true);
          return tr;
        },
        props: {
          handleClickOn: (_view, _position, node, _nodePosition, event) => {
            if (node.type.name !== 'footnoteReference') return false;
            event.preventDefault();
            return focusNote(node.attrs.label);
          },
          handleKeyDown: (_view, event) => {
            if (event.key !== 'Enter' && event.key !== ' ') return false;
            const target = event.target;
            if (!(target instanceof HTMLElement) || !target.matches('.composer-footnote-ref'))
              return false;
            event.preventDefault();
            return focusNote(target.dataset.footnoteLabel ?? '');
          }
        }
      })
    ];
  }
});

/** A block container preserves formatted paragraphs, lists, and code within a note. */
export const FootnoteDefinition = Node.create({
  name: 'footnoteDefinition',
  group: 'block',
  content: 'block+',
  defining: true,
  isolating: true,

  addAttributes() {
    return {
      label: { default: '', rendered: false },
      number: { default: 1, rendered: false }
    };
  },

  parseHTML() {
    return [
      {
        tag: 'div[data-footnote-label]',
        getAttrs: (node) => ({ label: node.dataset.footnoteLabel })
      }
    ];
  },

  renderHTML({ node }) {
    return [
      'div',
      {
        class: 'composer-footnote',
        'data-footnote-label': node.attrs.label,
        'data-footnote-number': node.attrs.number
      },
      0
    ];
  },

  renderText({ node }) {
    // Plain-text clipboard data must carry the definition as well as its markers.
    return (
      this.editor?.markdown?.serialize(node.toJSON()) ??
      `[^${node.attrs.label}]: ${node.textContent}`
    );
  },

  markdownTokenizer: {
    name: 'footnoteDefinition',
    level: 'block',
    start: (source) => source.search(/^ {0,3}\[\^[^\]\s]+\]:/m),
    tokenize(source, _tokens, lexer) {
      const token = tokenizeDefinition(source);
      if (token) return { ...token, tokens: lexer.blockTokens(token.text) };
    }
  },

  parseMarkdown(token, helpers) {
    // A definition inside a tight list inherits Marked's inline block state.
    // Convert those text tokens to paragraphs before creating the block container.
    const tokens = (token.tokens ?? []).map((child) =>
      child.type === 'text' ? { ...child, type: 'paragraph' } : child
    );
    const content = helpers.parseChildren(tokens);
    return {
      type: 'footnoteDefinition',
      attrs: { label: token.label },
      content: content.length ? content : [{ type: 'paragraph' }]
    };
  },

  renderMarkdown(node, helpers) {
    const content = helpers.renderChildren(node.content ?? [], '\n\n');
    return `[^${node.attrs?.label}]: ${content.replace(/\n/g, '\n    ')}`;
  }
});
