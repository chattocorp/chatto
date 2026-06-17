<!--
@component

Lightweight TipTap editor wrapper for chat input. Manages editor lifecycle
and exposes a typed API for text manipulation (mentions, emoji, drafts).

**Props:**
- `placeholder` - Placeholder text shown when editor is empty
- `editable` - Whether the editor accepts input
- `autofocus` - Focus editor on mount
- `testid` - data-testid attribute for E2E testing
- `onUpdate` - Called with markdown content on each change
- `onKeyDown` - Keyboard event handler; return true to prevent TipTap default
- `onPaste` - Paste event handler; return true to prevent TipTap default
- `onReady` - Called with editor API when editor is initialized
-->
<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { Editor, Extension, InputRule, mergeAttributes } from '@tiptap/core';
  import type { Node as ProseMirrorNode, Schema } from '@tiptap/pm/model';
  import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
  import StarterKit from '@tiptap/starter-kit';
  import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
  import { Markdown } from '@tiptap/markdown';
  import Placeholder from '@tiptap/extension-placeholder';
  import {
    CODE_LANGUAGE_OPTIONS,
    ensureCodeLanguagesLoaded,
    lowlight
  } from '$lib/codeHighlighting';

  const markdownLinkInputRegex = /(^|\s)\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)$/;
  const codeFenceLineRegex = /^```([\w-]+)?$/;
  const markdownBulletListLineRegex = /^[ \t]{0,3}[-+*]\s(.*)$/;
  const markdownOrderedListLineRegex = /^[ \t]{0,3}(\d{1,9})[.)]\s(.*)$/;

  const ComposerCodeBlockLowlight = CodeBlockLowlight.extend({
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

  const MarkdownLinkInputRule = Extension.create({
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

  const MarkdownCodeFenceShortcut = Extension.create({
    name: 'markdownCodeFenceShortcut',
    priority: 1000,

    addKeyboardShortcuts() {
      return {
        'Shift-Enter': () => {
          const { editor } = this;
          if (editor.isActive('codeBlock')) {
            return editor.commands.exitCode();
          }

          const { selection } = editor.state;
          const { empty } = selection;
          const fromPos = selection.$from;
          if (!empty || fromPos.parent.type.name !== 'paragraph') return false;
          if (fromPos.depth !== 1) return false;

          const textBeforeCursor = fromPos.parent.textBetween(0, fromPos.parentOffset, '\n', '\n');
          const textAfterCursor = fromPos.parent.textBetween(
            fromPos.parentOffset,
            fromPos.parent.content.size,
            '\n',
            '\n'
          );
          const currentLine = textBeforeCursor.split('\n').at(-1) ?? '';
          if (textAfterCursor && !textAfterCursor.startsWith('\n')) return false;

          const match = currentLine.match(codeFenceLineRegex);
          if (!match) return false;

          const paragraphPos = fromPos.before(1);
          const currentLineIndex = textBeforeCursor.split('\n').length - 1;
          const appendTrailingParagraph =
            paragraphPos + fromPos.parent.nodeSize === editor.state.doc.content.size;
          const replacement = buildCodeFenceReplacement({
            schema: editor.state.schema,
            paragraph: fromPos.parent,
            openLineIndex: currentLineIndex,
            closeLineIndex: null,
            appendTrailingParagraph
          });
          if (!replacement) return false;

          const codePosition = paragraphPos + replacement.beforeNodeSize;
          const tr = editor.state.tr.replaceWith(
            paragraphPos,
            paragraphPos + fromPos.parent.nodeSize,
            replacement.nodes
          );
          tr.setSelection(TextSelection.create(tr.doc, codePosition + 1));
          editor.view.dispatch(tr.scrollIntoView());
          return true;
        },

        Enter: () => {
          const { editor } = this;
          if (!editor.isActive('codeBlock')) return false;
          return editor.commands.newlineInCode();
        }
      };
    }
  });

  const CompletedMarkdownCodeFence = Extension.create({
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
                    currentParagraphPos +
                    replacement.beforeNodeSize +
                    replacement.codeNode.nodeSize;
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

  const MarkdownListMarkerAfterHardBreak = Extension.create({
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
            const textBeforeCursor = paragraph.textBetween(
              0,
              selectionFrom.parentOffset,
              '\n',
              '\n'
            );
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

  const TrailingParagraphAfterCodeBlock = Extension.create({
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

  function encodeMarkdownTextHtml(text: string): string {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;');
  }

  function decodeSerializedTextEntities(text: string): string {
    return text
      .split(/(\n)/)
      .map((part) => {
        if (part === '\n') return part;

        const leadingBlockquoteMarker = part.match(/^( {0,3})&gt;(?=\s|$)/);
        const protectedPart = leadingBlockquoteMarker
          ? `${leadingBlockquoteMarker[1]}__CHATTO_LITERAL_BLOCKQUOTE_MARKER__${part.slice(leadingBlockquoteMarker[0].length)}`
          : part;

        return protectedPart
          .replace(/&lt;/g, '<')
          .replace(/&gt;/g, '>')
          .replace(/&amp;/g, '&')
          .replace('__CHATTO_LITERAL_BLOCKQUOTE_MARKER__', '&gt;');
      })
      .join('');
  }

  function transformOutsideMarkdownLinkDestinations(
    text: string,
    transformText: (text: string) => string
  ): string {
    let result = '';
    let index = 0;
    let textStart = 0;
    const bracketStack: number[] = [];

    while (index < text.length) {
      const char = text[index];
      if (char === '\\') {
        index += 2;
        continue;
      }

      if (char === '[') {
        bracketStack.push(index);
        index += 1;
        continue;
      }

      if (char !== ']' || text[index + 1] !== '(' || bracketStack.length === 0) {
        index += 1;
        continue;
      }

      bracketStack.pop();
      const destinationStart = index;
      const destinationContentStart = destinationStart + 2;
      let destinationEnd = destinationContentStart;
      let nestedParens = 0;
      while (destinationEnd < text.length) {
        const char = text[destinationEnd];
        if (char === '\\') {
          destinationEnd += 2;
          continue;
        }
        if (char === '(') {
          nestedParens += 1;
        } else if (char === ')') {
          if (nestedParens === 0) break;
          nestedParens -= 1;
        }
        destinationEnd += 1;
      }

      if (destinationEnd >= text.length) {
        result += transformOutsideMarkdownAutolinks(text.slice(textStart), transformText);
        return result;
      }

      result += transformOutsideMarkdownAutolinks(
        text.slice(textStart, destinationContentStart),
        transformText
      );
      result += text.slice(destinationContentStart, destinationEnd + 1);
      index = destinationEnd + 1;
      textStart = index;
    }

    result += transformOutsideMarkdownAutolinks(text.slice(textStart), transformText);
    return result;
  }

  function transformOutsideMarkdownAutolinks(
    text: string,
    transformText: (text: string) => string
  ): string {
    let result = '';
    let index = 0;
    const autolinkPattern = /<https?:\/\/[^\s<>]+>/gi;

    for (const match of text.matchAll(autolinkPattern)) {
      result += transformText(text.slice(index, match.index));
      result += match[0];
      index = match.index + match[0].length;
    }

    result += transformText(text.slice(index));
    return result;
  }

  function transformMarkdownTextSegment(
    text: string,
    transformText: (text: string) => string,
    { skipLinkDestinations = false }: { skipLinkDestinations?: boolean } = {}
  ): string {
    return skipLinkDestinations
      ? transformOutsideMarkdownLinkDestinations(text, transformText)
      : transformText(text);
  }

  function transformOutsideInlineCode(
    line: string,
    transformText: (text: string) => string,
    options: { skipLinkDestinations?: boolean } = {}
  ): string {
    let result = '';
    let index = 0;

    while (index < line.length) {
      const codeStart = line.indexOf('`', index);
      if (codeStart === -1) {
        result += transformMarkdownTextSegment(line.slice(index), transformText, options);
        break;
      }

      result += transformMarkdownTextSegment(line.slice(index, codeStart), transformText, options);

      let delimiterEnd = codeStart + 1;
      while (line[delimiterEnd] === '`') delimiterEnd += 1;

      const delimiter = line.slice(codeStart, delimiterEnd);
      const codeEnd = line.indexOf(delimiter, delimiterEnd);
      if (codeEnd === -1) {
        result += transformMarkdownTextSegment(line.slice(codeStart), transformText, options);
        break;
      }

      result += line.slice(codeStart, codeEnd + delimiter.length);
      index = codeEnd + delimiter.length;
    }

    return result;
  }

  function transformMarkdownOutsideCode(
    markdown: string,
    transformText: (text: string) => string,
    options: { skipLinkDestinations?: boolean } = {}
  ): string {
    const lines = markdown.match(/[^\n]*(?:\n|$)/g) ?? [];
    if (lines[lines.length - 1] === '') {
      lines.pop();
    }

    let result = '';
    let pendingText = '';
    let inFence = false;
    let fenceChar = '';
    let fenceLength = 0;
    let canStartIndentedCode = true;

    const flushPendingText = () => {
      if (!pendingText) return;
      result += transformOutsideInlineCode(pendingText, transformText, options);
      pendingText = '';
    };

    for (const lineWithBreak of lines) {
      const hasLineBreak = lineWithBreak.endsWith('\n');
      const line = hasLineBreak ? lineWithBreak.slice(0, -1) : lineWithBreak;
      const blockquoteContent = line.replace(/^(?: {0,3}> ?)+/, '');

      if (/^ *$/.test(blockquoteContent)) {
        if (inFence) {
          result += lineWithBreak;
        } else {
          pendingText += lineWithBreak;
        }
        canStartIndentedCode = true;
        continue;
      }

      const fence = blockquoteContent.match(/^ {0,3}(`{3,}|~{3,})/);
      if (fence) {
        flushPendingText();
        const marker = fence[1];
        if (!inFence) {
          inFence = true;
          fenceChar = marker[0];
          fenceLength = marker.length;
        } else if (
          marker[0] === fenceChar &&
          marker.length >= fenceLength &&
          new RegExp(`^ {0,3}\\${fenceChar}{${fenceLength},} *$`).test(blockquoteContent)
        ) {
          inFence = false;
          fenceChar = '';
          fenceLength = 0;
        }

        result += lineWithBreak;
        canStartIndentedCode = true;
        continue;
      }

      if (inFence) {
        result += lineWithBreak;
        continue;
      }

      if (canStartIndentedCode && /^( {4,}|\t)/.test(blockquoteContent)) {
        flushPendingText();
        result += lineWithBreak;
        continue;
      }

      pendingText += lineWithBreak;
      canStartIndentedCode = false;
    }

    flushPendingText();
    return result;
  }

  function escapeMarkdownHtml(markdown: string): string {
    return transformMarkdownOutsideCode(markdown, encodeMarkdownTextHtml, {
      skipLinkDestinations: true
    });
  }

  function decodeSerializedMarkdownText(markdown: string): string {
    return transformMarkdownOutsideCode(markdown, decodeSerializedTextEntities);
  }

  function hasTrailingEmptyParagraph(e: Editor): boolean {
    if (e.state.doc.childCount <= 1) return false;
    const lastChild = e.state.doc.lastChild;
    return lastChild?.type.name === 'paragraph' && lastChild.content.size === 0;
  }

  function trimSerializedTrailingEmptyParagraph(markdown: string, e: Editor): string {
    if (!hasTrailingEmptyParagraph(e)) return markdown;
    return markdown.replace(/(?:\n\n(?:&nbsp;|\u00a0))+$/, '');
  }

  function normalizeSerializedHardBreaksBeforeLists(markdown: string): string {
    return markdown.replace(/ {2,}(\n\s*\n\s*(?:[-+*]|\d{1,9}[.)])\s)/g, '$1');
  }

  function getSerializedMarkdown(e: Editor): string {
    return normalizeSerializedHardBreaksBeforeLists(
      trimSerializedTrailingEmptyParagraph(decodeSerializedMarkdownText(e.getMarkdown()), e)
    );
  }

  export type TipTapEditorApi = {
    /** Get the editor's plain text content */
    getText: () => string;
    /** Set editor content from markdown */
    setContent: (markdown: string) => void;
    /** Focus the editor */
    focus: (position?: 'start' | 'end') => void;
    /** Get plain text from document start to cursor position */
    getTextBeforeCursor: () => string;
    /** Whether the current selection is inside a code block */
    isInCodeBlock: () => boolean;
    /** Whether the current selection is inside a top-level plain paragraph */
    isInPlainParagraph: () => boolean;
    /**
     * Replace N characters before the cursor with new text.
     * Used for mention/emoji completion where we know the pattern
     * length relative to the cursor.
     */
    replaceTextBeforeCursor: (charCount: number, replacement: string) => void;
  };

  let {
    placeholder = 'Type a message...',
    editable = true,
    autofocus = false,
    testid,
    onUpdate,
    onKeyDown,
    onPaste,
    onReady
  }: {
    placeholder?: string;
    editable?: boolean;
    autofocus?: boolean;
    testid?: string;
    onUpdate?: (text: string) => void;
    onKeyDown?: (event: KeyboardEvent) => boolean;
    onPaste?: (event: ClipboardEvent) => boolean;
    onReady?: (api: TipTapEditorApi) => void;
  } = $props();

  let editorElement = $state<HTMLDivElement>();
  let editorFrameElement = $state<HTMLDivElement>();
  let editor = $state<Editor | null>(null);
  let activeCodeBlockLanguage = $state<string | null>(null);
  let activeCodeBlockSelectorPosition = $state<{ right: number; bottom: number } | null>(null);
  let activeLinkHref = $state<string | null>(null);
  let activeLinkRange = $state<{ from: number; to: number } | null>(null);
  let linkHrefDraft = $state('');
  let linkDraftInitializedFor = $state<string | null>(null);
  let codeLanguageLoadToken = 0;

  let hasLinkControls = $derived(activeLinkHref !== null);
  let activeCodeBlockLanguageLabel = $derived(
    CODE_LANGUAGE_OPTIONS.find((language) => language.value === activeCodeBlockLanguage)?.label ??
      activeCodeBlockLanguage?.toUpperCase() ??
      'TEXT'
  );
  let codeLanguageSelectStyle = $derived(
    activeCodeBlockSelectorPosition
      ? `right: ${activeCodeBlockSelectorPosition.right}px; bottom: ${activeCodeBlockSelectorPosition.bottom}px;`
      : ''
  );

  function getAdjacentLinkRange(e: Editor) {
    const linkType = e.state.schema.marks.link;
    const { selection } = e.state;
    if (!linkType || !selection.empty) return null;

    const fromPos = selection.$from;
    const adjacentNodes = [
      { node: fromPos.nodeBefore, from: fromPos.pos - (fromPos.nodeBefore?.nodeSize ?? 0) },
      { node: fromPos.nodeAfter, from: fromPos.pos }
    ];

    for (const { node, from } of adjacentNodes) {
      const mark = node?.marks.find((m) => m.type === linkType);
      if (node && mark) {
        return { href: mark.attrs.href ?? '', range: { from, to: from + node.nodeSize } };
      }
    }

    return null;
  }

  function updateActiveCodeBlockSelectorPosition(e: Editor) {
    if (!editorFrameElement || !e.isActive('codeBlock')) {
      activeCodeBlockSelectorPosition = null;
      return;
    }

    const selectionFrom = e.state.selection.$from;
    let codeBlockDepth = 0;
    for (let depth = selectionFrom.depth; depth > 0; depth -= 1) {
      if (selectionFrom.node(depth).type.name === 'codeBlock') {
        codeBlockDepth = depth;
        break;
      }
    }

    if (!codeBlockDepth) {
      activeCodeBlockSelectorPosition = null;
      return;
    }

    const codeBlockPosition = selectionFrom.before(codeBlockDepth);
    const nodeDom = e.view.nodeDOM(codeBlockPosition);
    const preElement =
      nodeDom instanceof HTMLElement
        ? nodeDom.tagName === 'PRE'
          ? nodeDom
          : nodeDom.closest('pre')
        : null;
    if (!preElement) {
      activeCodeBlockSelectorPosition = null;
      return;
    }

    const frameRect = editorFrameElement.getBoundingClientRect();
    const preRect = preElement.getBoundingClientRect();
    activeCodeBlockSelectorPosition = {
      right: frameRect.right - preRect.right,
      bottom: frameRect.bottom - preRect.bottom
    };
  }

  function updateActiveControls(e: Editor) {
    if (e.isActive('codeBlock')) {
      activeCodeBlockLanguage = e.getAttributes('codeBlock').language || 'text';
    } else {
      activeCodeBlockLanguage = null;
    }
    updateActiveCodeBlockSelectorPosition(e);

    const adjacentLink = getAdjacentLinkRange(e);

    if (e.isActive('link') || adjacentLink) {
      const href = adjacentLink?.href ?? e.getAttributes('link').href ?? '';
      activeLinkHref = href;
      activeLinkRange = adjacentLink?.range ?? null;
      if (linkDraftInitializedFor !== href) {
        linkHrefDraft = href;
        linkDraftInitializedFor = href;
      }
    } else {
      activeLinkHref = null;
      activeLinkRange = null;
      linkHrefDraft = '';
      linkDraftInitializedFor = null;
    }
  }

  function setCodeBlockLanguage(language: string) {
    if (!editor) return;

    editor
      .chain()
      .focus()
      .updateAttributes('codeBlock', { language: language || null })
      .run();
    updateActiveControls(editor);
    ensureEditorCodeLanguages(editor);
  }

  function getEditorCodeBlockLanguages(e: Editor): string[] {
    const languages: string[] = [];

    e.state.doc.descendants((node) => {
      if (node.type.name === 'codeBlock') {
        const language = node.attrs.language || 'text';
        if (!languages.includes(language)) {
          languages.push(language);
        }
      }
    });

    return languages;
  }

  function refreshCodeBlockDecorations(e: Editor) {
    if (e.isDestroyed) return;

    let tr = e.state.tr;
    e.state.doc.descendants((node, pos) => {
      if (node.type.name === 'codeBlock') {
        tr = tr.setNodeMarkup(pos, undefined, node.attrs, node.marks);
      }
    });

    if (tr.steps.length > 0) {
      e.view.dispatch(tr);
    }
  }

  function ensureEditorCodeLanguages(e: Editor) {
    const languages = getEditorCodeBlockLanguages(e);
    if (languages.length === 0) return;

    const loadToken = ++codeLanguageLoadToken;
    ensureCodeLanguagesLoaded(languages).then((loadedNewLanguage) => {
      if (
        !loadedNewLanguage ||
        e.isDestroyed ||
        editor !== e ||
        loadToken !== codeLanguageLoadToken
      ) {
        return;
      }

      refreshCodeBlockDecorations(e);
      updateActiveControls(e);
    });
  }

  function normalizeHref(href: string) {
    const trimmed = href.trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed;
    return `https://${trimmed}`;
  }

  function applyLinkHref() {
    if (!editor || activeLinkHref === null) return;

    const href = normalizeHref(linkHrefDraft);
    if (!href) {
      removeLink();
      return;
    }

    const linkType = editor.state.schema.marks.link;
    if (activeLinkRange && linkType) {
      const tr = editor.state.tr.addMark(
        activeLinkRange.from,
        activeLinkRange.to,
        linkType.create({ href })
      );
      editor.view.dispatch(tr);
      editor.commands.focus();
    } else {
      editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
    }
    updateActiveControls(editor);
  }

  function removeLink() {
    if (!editor) return;

    const linkType = editor.state.schema.marks.link;
    if (activeLinkRange && linkType) {
      const tr = editor.state.tr.removeMark(activeLinkRange.from, activeLinkRange.to, linkType);
      editor.view.dispatch(tr);
      editor.commands.focus();
    } else {
      editor.chain().focus().extendMarkRange('link').unsetLink().run();
    }
    updateActiveControls(editor);
  }

  function openActiveLink() {
    const href = normalizeHref(activeLinkHref ?? '');
    if (!href) return;

    window.open(href, '_blank', 'noopener,noreferrer');
  }

  function buildApi(e: Editor): TipTapEditorApi {
    const syncControls = () => {
      if (e.isDestroyed) return;
      updateActiveControls(e);
    };

    return {
      getText: () => (e.isDestroyed ? '' : e.getText({ blockSeparator: '\n' })),

      setContent: (markdown: string) => {
        if (e.isDestroyed) return;
        e.commands.setContent(escapeMarkdownHtml(markdown), {
          contentType: 'markdown',
          emitUpdate: false
        });
        ensureEditorCodeLanguages(e);
        tick().then(syncControls);
      },

      focus: (position: 'start' | 'end' = 'end') => {
        if (e.isDestroyed) return;
        e.commands.focus(position);
        tick().then(syncControls);
      },

      getTextBeforeCursor: () => {
        if (e.isDestroyed) return '';
        const { from } = e.state.selection;
        return e.state.doc.textBetween(0, from, '\n');
      },

      isInCodeBlock: () => !e.isDestroyed && e.isActive('codeBlock'),
      isInPlainParagraph: () => {
        if (e.isDestroyed) return false;
        const selectionFrom = e.state.selection.$from;
        return selectionFrom.depth === 1 && selectionFrom.parent.type.name === 'paragraph';
      },

      replaceTextBeforeCursor: (charCount: number, replacement: string) => {
        if (e.isDestroyed) return;
        const { from } = e.state.selection;
        e.chain()
          .focus()
          .deleteRange({ from: from - charCount, to: from })
          .insertContent(replacement)
          .run();
        tick().then(syncControls);
      }
    };
  }

  // Create and destroy editor with the DOM element lifecycle.
  // Only editorElement should trigger recreation — all other props are
  // handled by the incremental-update effects below, so we untrack them
  // to avoid destroying/recreating the editor on prop changes.
  $effect(() => {
    if (!editorElement) return;

    const e = untrack(
      () =>
        new Editor({
          element: editorElement,
          extensions: [
            StarterKit.configure({
              // Keep the composer subset aligned with the rendered message markdown.
              codeBlock: false,
              strike: false,
              underline: false,
              horizontalRule: false,
              link: {
                openOnClick: false,
                enableClickSelection: true
              }
            }),
            Markdown.configure({
              markedOptions: {
                breaks: true
              }
            }),
            ComposerCodeBlockLowlight.configure({ lowlight }),
            MarkdownLinkInputRule,
            MarkdownCodeFenceShortcut,
            CompletedMarkdownCodeFence,
            MarkdownListMarkerAfterHardBreak,
            TrailingParagraphAfterCodeBlock,
            Placeholder.configure({ placeholder })
          ],
          content: '',
          contentType: 'markdown',
          editable,
          autofocus: autofocus ? 'end' : false,
          editorProps: {
            attributes: testid ? { 'data-testid': testid } : {},
            handleKeyDown: (_view, event) => {
              return onKeyDown?.(event) ?? false;
            },
            handlePaste: (_view, event) => {
              return onPaste?.(event) ?? false;
            }
          },
          onUpdate: ({ editor: ed }) => {
            updateActiveControls(ed);
            ensureEditorCodeLanguages(ed);
            onUpdate?.(ed.isEmpty ? '' : getSerializedMarkdown(ed));
          },
          onSelectionUpdate: ({ editor: ed }) => {
            updateActiveControls(ed);
          }
        })
    );

    editor = e;

    // Notify parent that editor is ready with API
    tick().then(() => {
      if (e.isDestroyed || editor !== e) return;
      onReady?.(buildApi(e));
    });

    return () => {
      editor?.destroy();
      editor = null;
    };
  });

  // React to editable prop changes
  $effect(() => {
    if (editor && editor.isEditable !== editable) {
      editor.setEditable(editable);
    }
  });

  // React to placeholder prop changes (e.g., switching between normal and edit mode)
  $effect(() => {
    if (!editor) return;
    const ext = editor.extensionManager.extensions.find((e) => e.name === 'placeholder');
    if (ext && ext.options.placeholder !== placeholder) {
      ext.options.placeholder = placeholder;
      // Force ProseMirror to re-render decorations with the new placeholder
      editor.view.dispatch(editor.state.tr);
    }
  });
</script>

<svelte:window onresize={() => editor && updateActiveControls(editor)} />

<div bind:this={editorFrameElement} class="relative flex min-w-0 flex-1 flex-col gap-1">
  {#if hasLinkControls}
    <div class="flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted">
      <div class="flex min-w-0 items-center gap-1">
        <input
          aria-label="Link URL"
          title="Link URL"
          value={linkHrefDraft}
          disabled={!editable}
          oninput={(event) => (linkHrefDraft = event.currentTarget.value)}
          onkeydown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              applyLinkHref();
            }
          }}
          onblur={applyLinkHref}
          class="h-6 w-48 min-w-0 rounded border border-border bg-surface-200 px-2 text-xs text-text outline-none hover:bg-surface-300 focus:border-accent disabled:cursor-not-allowed disabled:opacity-50"
        />
        <button
          type="button"
          aria-label="Open link"
          title="Open link"
          disabled={!activeLinkHref}
          onclick={openActiveLink}
          class="flex h-6 w-6 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-300 hover:text-text"
        >
          <span class="iconify text-base uil--external-link-alt"></span>
        </button>
        <button
          type="button"
          aria-label="Remove link"
          title="Remove link"
          disabled={!editable}
          onclick={removeLink}
          class="flex h-6 w-6 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-300 hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span class="iconify text-base uil--link-broken"></span>
        </button>
      </div>
    </div>
  {/if}

  <div
    bind:this={editorElement}
    onscroll={() => editor && updateActiveControls(editor)}
    class={[
      'tiptap-editor max-h-50 min-h-8 min-w-0 flex-1 overflow-x-hidden overflow-y-auto bg-transparent py-1 text-text',
      !editable && 'cursor-not-allowed'
    ]}
  ></div>

  {#if activeCodeBlockLanguage !== null && activeCodeBlockSelectorPosition}
    <div class="absolute z-10" style={codeLanguageSelectStyle}>
      <div
        class="group relative inline-flex h-6 items-center gap-1 rounded-tl-md rounded-br-md bg-surface-200 pr-1.5 pl-2 font-mono text-xs tracking-wide text-muted uppercase focus-within:bg-surface-300 focus-within:text-text focus-within:ring-1 focus-within:ring-accent hover:bg-surface-300 hover:text-text"
      >
        <span>{activeCodeBlockLanguageLabel}</span>
        <span class="iconify size-3 uil--angle-down"></span>
        <select
          aria-label="Code language"
          title="Code language"
          value={activeCodeBlockLanguage}
          disabled={!editable}
          onchange={(event) => setCodeBlockLanguage(event.currentTarget.value)}
          class="absolute inset-0 h-full w-full cursor-pointer opacity-0 disabled:cursor-not-allowed"
        >
          {#each CODE_LANGUAGE_OPTIONS as language (language.value)}
            <option value={language.value}>{language.label}</option>
          {/each}
        </select>
      </div>
    </div>
  {/if}
</div>

<style>
  /* ProseMirror needs explicit outline removal and placeholder styling
	   that can't be achieved with Tailwind alone (pseudo-elements) */
  :global(.tiptap-editor .ProseMirror) {
    outline: none;
    word-break: break-word;
    font-size: 16px; /* Prevent iOS Safari auto-zoom on focus */
    line-height: 1.5;
  }

  :global(.tiptap-editor .ProseMirror p),
  :global(.tiptap-editor .ProseMirror blockquote),
  :global(.tiptap-editor .ProseMirror ul),
  :global(.tiptap-editor .ProseMirror ol),
  :global(.tiptap-editor .ProseMirror pre),
  :global(.tiptap-editor .ProseMirror h1),
  :global(.tiptap-editor .ProseMirror h2),
  :global(.tiptap-editor .ProseMirror h3),
  :global(.tiptap-editor .ProseMirror h4),
  :global(.tiptap-editor .ProseMirror h5),
  :global(.tiptap-editor .ProseMirror h6) {
    margin: 0;
  }

  :global(.tiptap-editor .ProseMirror > * + *) {
    margin-top: 0.5em;
  }

  :global(.tiptap-editor .ProseMirror strong) {
    font-weight: 600;
  }

  :global(.tiptap-editor .ProseMirror a) {
    color: var(--color-link);
    text-decoration: underline;
    text-underline-offset: 2px;
    overflow-wrap: anywhere;
  }

  :global(.tiptap-editor .ProseMirror ul),
  :global(.tiptap-editor .ProseMirror ol) {
    padding-left: 1.5em;
  }

  :global(.tiptap-editor .ProseMirror ul) {
    list-style-type: disc;
  }

  :global(.tiptap-editor .ProseMirror ol) {
    list-style-type: decimal;
  }

  :global(.tiptap-editor .ProseMirror blockquote) {
    border-left: 3px solid var(--color-border);
    padding-left: 1em;
    color: var(--color-muted);
    font-style: italic;
  }

  :global(.tiptap-editor .ProseMirror code:not(pre code)) {
    border-radius: 0.25rem;
    background: var(--color-surface-200);
    padding: 0.125rem 0.375rem;
    font-family: var(--font-mono);
    font-size: 0.9em;
  }

  :global(.tiptap-editor .ProseMirror pre) {
    overflow: hidden;
    position: relative;
    width: 100%;
    border-radius: 0.375rem;
    border: 1px solid var(--color-surface-200);
    background: transparent;
    padding: 0.5rem 0.75rem;
    font-family: var(--font-mono);
    font-size: 0.875rem;
    line-height: 1.5;
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
  }

  :global(.tiptap-editor .ProseMirror > pre) {
    margin-block: 0.5rem;
  }

  :global(.tiptap-editor .ProseMirror > pre:first-child) {
    margin-top: 0;
  }

  :global(.tiptap-editor .ProseMirror > pre:last-child) {
    margin-bottom: 0;
  }

  :global(.tiptap-editor .ProseMirror pre[data-language]::after) {
    content: attr(data-language);
    position: absolute;
    right: 0;
    bottom: 0;
    border-top-left-radius: 0.375rem;
    background: var(--color-surface-200);
    padding: 0.125rem 0.5rem;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    line-height: 1rem;
    letter-spacing: 0.025em;
    color: var(--color-muted);
    text-transform: uppercase;
    pointer-events: none;
  }

  :global(.tiptap-editor .ProseMirror pre code) {
    display: block;
    overflow-x: auto;
    background: transparent;
    padding: 0 3.5rem 0 0;
    font-size: inherit;
    line-height: inherit;
    color: var(--composer-code-text);
    white-space: pre;
  }

  :global(.tiptap-editor) {
    --composer-code-text: #24292f;
    --composer-code-comment: #6e7781;
    --composer-code-keyword: #cf222e;
    --composer-code-string: #0a3069;
    --composer-code-title: #8250df;
    --composer-code-literal: #0550ae;
    --composer-code-attribute: #953800;
  }

  :global(:root[data-theme='dark'] .tiptap-editor) {
    --composer-code-text: #d0d7de;
    --composer-code-comment: #8b949e;
    --composer-code-keyword: #ff7b72;
    --composer-code-string: #a5d6ff;
    --composer-code-title: #d2a8ff;
    --composer-code-literal: #79c0ff;
    --composer-code-attribute: #ffa657;
  }

  :global(.tiptap-editor .ProseMirror .hljs-comment),
  :global(.tiptap-editor .ProseMirror .hljs-quote) {
    color: var(--composer-code-comment);
    font-style: italic;
  }

  :global(.tiptap-editor .ProseMirror .hljs-keyword),
  :global(.tiptap-editor .ProseMirror .hljs-selector-tag),
  :global(.tiptap-editor .ProseMirror .hljs-subst) {
    color: var(--composer-code-keyword);
  }

  :global(.tiptap-editor .ProseMirror .hljs-string),
  :global(.tiptap-editor .ProseMirror .hljs-regexp),
  :global(.tiptap-editor .ProseMirror .hljs-symbol),
  :global(.tiptap-editor .ProseMirror .hljs-bullet) {
    color: var(--composer-code-string);
  }

  :global(.tiptap-editor .ProseMirror .hljs-title),
  :global(.tiptap-editor .ProseMirror .hljs-section),
  :global(.tiptap-editor .ProseMirror .hljs-name),
  :global(.tiptap-editor .ProseMirror .hljs-selector-id),
  :global(.tiptap-editor .ProseMirror .hljs-selector-class) {
    color: var(--composer-code-title);
  }

  :global(.tiptap-editor .ProseMirror .hljs-number),
  :global(.tiptap-editor .ProseMirror .hljs-literal),
  :global(.tiptap-editor .ProseMirror .hljs-type),
  :global(.tiptap-editor .ProseMirror .hljs-built_in) {
    color: var(--composer-code-literal);
  }

  :global(.tiptap-editor .ProseMirror .hljs-attr),
  :global(.tiptap-editor .ProseMirror .hljs-attribute),
  :global(.tiptap-editor .ProseMirror .hljs-variable),
  :global(.tiptap-editor .ProseMirror .hljs-template-variable) {
    color: var(--composer-code-attribute);
  }

  :global(.tiptap-editor .ProseMirror h1),
  :global(.tiptap-editor .ProseMirror h2),
  :global(.tiptap-editor .ProseMirror h3),
  :global(.tiptap-editor .ProseMirror h4),
  :global(.tiptap-editor .ProseMirror h5),
  :global(.tiptap-editor .ProseMirror h6) {
    font-size: inherit;
    font-weight: 600;
    text-decoration: underline;
  }

  /* Placeholder styling via the Placeholder extension */
  :global(.tiptap-editor .ProseMirror p.is-editor-empty:first-child::before) {
    content: attr(data-placeholder);
    float: left;
    pointer-events: none;
    height: 0;
    color: var(--color-muted);
  }
</style>
