import { Extension, InputRule, mergeAttributes } from '@tiptap/core';
import type { Node as ProseMirrorNode, Schema } from '@tiptap/pm/model';
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { Markdown } from '@tiptap/markdown';
import Placeholder from '@tiptap/extension-placeholder';
import { lowlight } from '$lib/codeHighlighting';
import { isDefaultEmptyDocument } from './markdown';

const markdownLinkInputRegex = /(^|\s)\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)$/;
const codeFenceLineRegex = /^```([\w-]+)?$/;
const markdownBulletListLineRegex = /^[ \t]{0,3}[-+*]\s(.*)$/;
const markdownOrderedListLineRegex = /^[ \t]{0,3}(\d{1,9})[.)]\s(.*)$/;

export const ComposerCodeBlockLowlight = CodeBlockLowlight.extend({
  renderHTML({ node, HTMLAttributes }) {
    const language = node.attrs.language || 'text';

    return [
      'pre',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, {
        'data-language': language
      }),
      [
        'code',
        {
          class: language ? this.options.languageClassPrefix + language : null
        },
        0
      ]
    ];
  }
});

export const ComposerLink = Link.extend({ inclusive: false });

const SelectedTextInlineCodeShortcut = Extension.create({
  name: 'selectedTextInlineCodeShortcut',

  addKeyboardShortcuts() {
    return {
      '`': () =>
        this.editor.state.selection.empty ? false : this.editor.commands.toggleCode()
    };
  }
});

function paragraphTextWithLineBreaks(node: ProseMirrorNode) {
  return node.textBetween(0, node.content.size, '\n', '\n');
}

function createParagraphFromText(schema: Schema, text: string) {
  const paragraph = schema.nodes.paragraph;
  const hardBreak = schema.nodes.hardBreak;
  if (!text) return paragraph.create();

  const content = text.split('\n').flatMap((line, index, lines) => {
    const nodes = [];
    if (line) nodes.push(schema.text(line));
    if (index < lines.length - 1 && hardBreak) nodes.push(hardBreak.create());
    return nodes;
  });

  return paragraph.create(null, content);
}

function buildCodeFenceReplacement({
  schema,
  paragraph,
  openLineIndex,
  closeLineIndex,
  appendTrailingParagraph
}: {
  schema: Schema;
  paragraph: Parameters<typeof paragraphTextWithLineBreaks>[0];
  openLineIndex: number;
  closeLineIndex: number | null;
  appendTrailingParagraph: boolean;
}) {
  const text = paragraphTextWithLineBreaks(paragraph);
  const lines = text.split('\n');
  const openingMatch = lines[openLineIndex]?.match(codeFenceLineRegex);
  const codeBlock = schema.nodes.codeBlock;
  if (!openingMatch || !codeBlock) return null;

  const beforeText = lines.slice(0, openLineIndex).join('\n');
  const codeText =
    closeLineIndex === null ? '' : lines.slice(openLineIndex + 1, closeLineIndex).join('\n');
  const afterText =
    closeLineIndex === null
      ? lines.slice(openLineIndex + 1).join('\n')
      : lines.slice(closeLineIndex + 1).join('\n');
  const language = openingMatch[1] || null;

  const nodes = [];
  const beforeNode = beforeText ? createParagraphFromText(schema, beforeText) : null;
  if (beforeNode) nodes.push(beforeNode);
  const codeNode = codeBlock.create(
    language ? { language } : undefined,
    codeText ? schema.text(codeText) : null
  );
  nodes.push(codeNode);
  if (afterText) {
    nodes.push(createParagraphFromText(schema, afterText));
  } else if (appendTrailingParagraph) {
    nodes.push(schema.nodes.paragraph.create());
  }

  return { nodes, codeNode, beforeNodeSize: beforeNode?.nodeSize ?? 0 };
}

function buildListMarkerReplacement({
  schema,
  paragraph,
  markerLineIndex
}: {
  schema: Schema;
  paragraph: Parameters<typeof paragraphTextWithLineBreaks>[0];
  markerLineIndex: number;
}) {
  const text = paragraphTextWithLineBreaks(paragraph);
  const lines = text.split('\n');
  const markerLine = lines[markerLineIndex] ?? '';
  const bulletMatch = markerLine.match(markdownBulletListLineRegex);
  const orderedMatch = markerLine.match(markdownOrderedListLineRegex);
  if (!bulletMatch && !orderedMatch) return null;

  const listNodeType = bulletMatch ? schema.nodes.bulletList : schema.nodes.orderedList;
  const listItem = schema.nodes.listItem;
  const paragraphNode = schema.nodes.paragraph;
  if (!listNodeType || !listItem || !paragraphNode) return null;

  const itemText = bulletMatch ? (bulletMatch[1] ?? '') : (orderedMatch?.[2] ?? '');
  const beforeText = lines.slice(0, markerLineIndex).join('\n');
  const afterText = lines.slice(markerLineIndex + 1).join('\n');
  const beforeNode = beforeText ? createParagraphFromText(schema, beforeText) : null;
  const itemParagraph = itemText
    ? paragraphNode.create(null, schema.text(itemText))
    : paragraphNode.create();
  const listAttrs = orderedMatch ? { start: Number.parseInt(orderedMatch[1], 10) } : undefined;
  const listNode = listNodeType.create(listAttrs, listItem.create(null, itemParagraph));
  const nodes = [];

  if (beforeNode) nodes.push(beforeNode);
  nodes.push(listNode);
  if (afterText) nodes.push(createParagraphFromText(schema, afterText));

  return {
    nodes,
    itemText,
    listNode,
    beforeNodeSize: beforeNode?.nodeSize ?? 0
  };
}

export const MarkdownLinkInputRule = Extension.create({
  name: 'markdownLinkInputRule',

  addInputRules() {
    return [
      new InputRule({
        find: markdownLinkInputRegex,
        handler: ({ state, range, match }) => {
          const prefix = match[1] ?? '';
          const label = match[2];
          const href = match[3];
          const linkType = state.schema.marks.link;
          if (!label || !href || !linkType) return null;

          const from = range.from + prefix.length;
          const to = range.to;
          const tr = state.tr;

          tr.delete(from, to);
          tr.insertText(label, from);
          tr.addMark(from, from + label.length, linkType.create({ href }));
          tr.removeStoredMark(linkType);
        }
      })
    ];
  }
});

export const CompletedMarkdownCodeFence = Extension.create({
  name: 'completedMarkdownCodeFence',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('completedMarkdownCodeFence'),
        appendTransaction: (transactions, _oldState, newState) => {
          if (!transactions.some((transaction) => transaction.docChanged)) return null;

          let paragraphPos = 0;
          for (let index = 0; index < newState.doc.childCount; index += 1) {
            const paragraph = newState.doc.child(index);
            const currentParagraphPos = paragraphPos;
            paragraphPos += paragraph.nodeSize;
            if (paragraph.type.name !== 'paragraph') continue;

            const lines = paragraphTextWithLineBreaks(paragraph).split('\n');
            for (let openLineIndex = 0; openLineIndex < lines.length; openLineIndex += 1) {
              if (!codeFenceLineRegex.test(lines[openLineIndex] ?? '')) continue;

              for (
                let closeLineIndex = openLineIndex + 1;
                closeLineIndex < lines.length;
                closeLineIndex += 1
              ) {
                if (lines[closeLineIndex] !== '```') continue;

                const appendTrailingParagraph =
                  currentParagraphPos + paragraph.nodeSize === newState.doc.content.size &&
                  closeLineIndex === lines.length - 1;
                const replacement = buildCodeFenceReplacement({
                  schema: newState.schema,
                  paragraph,
                  openLineIndex,
                  closeLineIndex,
                  appendTrailingParagraph
                });
                if (!replacement) return null;

                const tr = newState.tr.replaceWith(
                  currentParagraphPos,
                  currentParagraphPos + paragraph.nodeSize,
                  replacement.nodes
                );
                const codeEnd =
                  currentParagraphPos + replacement.beforeNodeSize + replacement.codeNode.nodeSize;
                tr.setSelection(TextSelection.near(tr.doc.resolve(codeEnd + 1), 1));
                return tr;
              }
            }
          }

          return null;
        }
      })
    ];
  }
});

export const MarkdownListMarkerAfterHardBreak = Extension.create({
  name: 'markdownListMarkerAfterHardBreak',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('markdownListMarkerAfterHardBreak'),
        appendTransaction: (transactions, _oldState, newState) => {
          if (!transactions.some((transaction) => transaction.docChanged)) return null;

          const selectionFrom = newState.selection.$from;
          if (selectionFrom.depth !== 1 || selectionFrom.parent.type.name !== 'paragraph') {
            return null;
          }

          const paragraph = selectionFrom.parent;
          const paragraphPos = selectionFrom.before(1);
          const textBeforeCursor = paragraph.textBetween(0, selectionFrom.parentOffset, '\n', '\n');
          const currentLineIndex = textBeforeCursor.split('\n').length - 1;
          const currentLine = textBeforeCursor.split('\n').at(-1) ?? '';

          if (
            !markdownBulletListLineRegex.test(currentLine) &&
            !markdownOrderedListLineRegex.test(currentLine)
          ) {
            return null;
          }

          const replacement = buildListMarkerReplacement({
            schema: newState.schema,
            paragraph,
            markerLineIndex: currentLineIndex
          });
          if (!replacement) return null;

          const listPosition = paragraphPos + replacement.beforeNodeSize;
          const selectionPosition = listPosition + 3 + replacement.itemText.length;
          const tr = newState.tr.replaceWith(
            paragraphPos,
            paragraphPos + paragraph.nodeSize,
            replacement.nodes
          );
          tr.setSelection(TextSelection.create(tr.doc, selectionPosition));
          return tr;
        }
      })
    ];
  }
});

export const TrailingParagraphAfterCodeBlock = Extension.create({
  name: 'trailingParagraphAfterCodeBlock',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('trailingParagraphAfterCodeBlock'),
        appendTransaction: (_transactions, _oldState, newState) => {
          const paragraph = newState.schema.nodes.paragraph;
          const lastChild = newState.doc.lastChild;
          if (!paragraph || !lastChild || lastChild.type.name !== 'codeBlock') return null;

          return newState.tr.insert(newState.doc.content.size, paragraph.create());
        }
      })
    ];
  }
});

export const ClearMarksOnEmptyDocument = Extension.create({
  name: 'clearMarksOnEmptyDocument',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('clearMarksOnEmptyDocument'),
        appendTransaction: (transactions, _oldState, newState) => {
          if (!transactions.some((transaction) => transaction.docChanged)) return null;
          if (!isDefaultEmptyDocument(newState.doc)) return null;
          if ((newState.storedMarks?.length ?? 0) === 0) return null;

          return newState.tr.setStoredMarks([]);
        }
      })
    ];
  }
});

/** Build the complete, deliberately constrained extension set for the composer. */
export function createComposerExtensions(placeholder: string) {
  return [
    StarterKit.configure({
      // Keep the composer subset aligned with the rendered message markdown.
      codeBlock: false,
      strike: false,
      underline: false,
      horizontalRule: false,
      trailingNode: false,
      link: false
    }),
    ComposerLink.configure({
      autolink: true,
      linkOnPaste: true,
      openOnClick: false,
      enableClickSelection: true
    }),
    Markdown.configure({
      markedOptions: {
        breaks: true
      }
    }),
    ComposerCodeBlockLowlight.configure({ lowlight }),
    SelectedTextInlineCodeShortcut.configure(),
    MarkdownLinkInputRule.configure(),
    CompletedMarkdownCodeFence.configure(),
    MarkdownListMarkerAfterHardBreak.configure(),
    TrailingParagraphAfterCodeBlock.configure(),
    ClearMarksOnEmptyDocument.configure(),
    Placeholder.configure({ placeholder })
  ];
}
