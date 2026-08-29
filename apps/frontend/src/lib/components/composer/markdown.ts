import type { Editor, JSONContent } from '@tiptap/core';
import { Fragment, type Mark, type Node as ProseMirrorNode, type Schema } from '@tiptap/pm/model';
import type { SelectedQuoteBlock } from '$lib/state/room';

const markdownLinkPasteRegex = /\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/g;

/** Return whether text is one complete HTTP(S) Markdown autolink. */
export function isHttpMarkdownAutolink(text: string): boolean {
  return /^<https?:\/\/[^\s<>]+>$/i.test(text);
}

export function isDefaultEmptyDocument(doc: ProseMirrorNode): boolean {
  if (doc.childCount !== 1) return false;
  const firstChild = doc.firstChild;
  return firstChild?.type.name === 'paragraph' && firstChild.content.size === 0;
}

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

type MarkdownTransformOptions = {
  skipLinkDestinations?: boolean;
  preserveInlineCode?: boolean;
};

function transformMarkdownTextSegment(
  text: string,
  transformText: (text: string) => string,
  { skipLinkDestinations = false }: MarkdownTransformOptions = {}
): string {
  return skipLinkDestinations
    ? transformOutsideMarkdownLinkDestinations(text, transformText)
    : transformText(text);
}

function transformOutsideInlineCode(
  line: string,
  transformText: (text: string) => string,
  options: MarkdownTransformOptions = {}
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
  options: MarkdownTransformOptions = {}
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
    result +=
      options.preserveInlineCode === false
        ? transformMarkdownTextSegment(pendingText, transformText, options)
        : transformOutsideInlineCode(pendingText, transformText, options);
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

function hasUnescapedPipe(line: string): boolean {
  for (let index = 0; index < line.length; index++) {
    if (line[index] === '|' && line[index - 1] !== '\\') return true;
  }
  return false;
}

function isGfmTableDelimiter(line: string): boolean {
  const content = line.replace(/^(?: {0,3}> ?)+/, '').trim();
  const cells = content.split('|');
  if (cells[0]?.trim() === '') cells.shift();
  if (cells.at(-1)?.trim() === '') cells.pop();
  return cells.length > 0 && cells.every((cell) => /^:?-+:?$/.test(cell.trim()));
}

function escapeGfmTablesForEditor(markdown: string): string {
  return transformMarkdownOutsideCode(
    markdown,
    (text) => {
      const lines = text.split('\n');
      for (let index = 1; index < lines.length; index++) {
        const header = lines[index - 1].replace(/^(?: {0,3}> ?)+/, '').trim();
        if (!hasUnescapedPipe(header) || !isGfmTableDelimiter(lines[index])) continue;

        // The composer intentionally has no table node. Keep the source as
        // editable prose by making the delimiter invisible to TipTap's GFM
        // parser. The word joiner is invisible and is removed again when
        // serializing, so the delimiter remains visually unchanged.
        lines[index] = lines[index].replace('-', '-\u2060');
      }
      return lines.join('\n');
    },
    { preserveInlineCode: false }
  );
}

export function prepareMarkdownForEditor(markdown: string): string {
  return escapeGfmTablesForEditor(escapeMarkdownHtml(markdown));
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

function normalizeSerializedGfmTableHardBreaks(markdown: string): string {
  return transformMarkdownOutsideCode(
    markdown,
    (text) => {
      const lines = text.split('\n');
      for (let index = 1; index < lines.length; index++) {
        const header = lines[index - 1].replace(/ {2,}$/, '');
        const delimiter = lines[index].replace(/ {2,}$/, '');
        const unescapedDelimiter = delimiter.replace('\u2060', '');
        if (!hasUnescapedPipe(header) || !isGfmTableDelimiter(unescapedDelimiter)) continue;

        lines[index - 1] = header;
        lines[index] = unescapedDelimiter;
        for (let rowIndex = index + 1; rowIndex < lines.length; rowIndex++) {
          const row = lines[rowIndex].replace(/ {2,}$/, '');
          if (!hasUnescapedPipe(row)) break;
          lines[rowIndex] = row;
        }
      }
      return lines.join('\n');
    },
    { preserveInlineCode: false }
  );
}

function encodeSerializedHeadingClosingHashes(markdown: string): string {
  return transformMarkdownOutsideCode(
    markdown,
    (text) =>
      text.replace(
        /(^|\n)((?:[ \t]{0,3}>[ \t]?)*[ \t]{0,3}#{1,6}[ \t]+[^\n]*?)([ \t]+)(#{1,})([ \t]*)(?=\n|$)/g,
        (_match, lineStart, headingStart, separator, hashes, trailingWhitespace) =>
          `${lineStart}${headingStart}${separator}${hashes.replace(/#/g, '&#35;')}${trailingWhitespace}`
      ),
    { preserveInlineCode: false }
  );
}

export function getSerializedMarkdown(e: Editor): string {
  return normalizeSerializedHardBreaksBeforeLists(
    normalizeSerializedGfmTableHardBreaks(
      encodeSerializedHeadingClosingHashes(
        trimSerializedTrailingEmptyParagraph(decodeSerializedMarkdownText(e.getMarkdown()), e)
      )
    )
  );
}

export function applyDestinationMarks(
  node: ProseMirrorNode,
  marks: readonly Mark[]
): ProseMirrorNode {
  const children: ProseMirrorNode[] = [];

  node.forEach((child) => {
    if (child.isText) {
      const combinedMarks = marks.reduce((current, mark) => mark.addToSet(current), child.marks);
      children.push(child.mark(node.type.allowedMarks(combinedMarks)));
    } else {
      children.push(applyDestinationMarks(child, marks));
    }
  });

  return node.copy(Fragment.fromArray(children));
}

function createClipboardInlineContent(
  text: string,
  schema: Schema,
  destinationMarks: readonly Mark[]
): ProseMirrorNode[] {
  const linkType = schema.marks.link;
  if (!text || !linkType) return text ? [schema.text(text, destinationMarks)] : [];

  const nodes: ProseMirrorNode[] = [];
  const nonLinkMarks = destinationMarks.filter((mark) => mark.type !== linkType);
  let index = 0;

  for (const match of text.matchAll(markdownLinkPasteRegex)) {
    const matchIndex = match.index;
    const label = match[1];
    const href = match[2];
    if (!label || !href || text[matchIndex - 1] === '!' || text[matchIndex - 1] === '\\') {
      continue;
    }

    if (matchIndex > index)
      nodes.push(schema.text(text.slice(index, matchIndex), destinationMarks));
    nodes.push(schema.text(label, [...nonLinkMarks, linkType.create({ href })]));
    index = matchIndex + match[0].length;
  }

  if (index < text.length) nodes.push(schema.text(text.slice(index), destinationMarks));
  return nodes;
}

export function createClipboardContent(
  text: string,
  schema: Schema,
  destinationMarks: readonly Mark[]
): Fragment {
  const paragraphType = schema.nodes.paragraph;
  const hardBreakType = schema.nodes.hardBreak;
  const paragraphMarks = paragraphType.allowedMarks(destinationMarks);

  return Fragment.fromArray(
    text.split(/\n{2,}/).map((paragraphText) => {
      const inlineNodes: ProseMirrorNode[] = [];
      const lines = paragraphText.split('\n');

      lines.forEach((line, index) => {
        inlineNodes.push(...createClipboardInlineContent(line, schema, paragraphMarks));
        if (index < lines.length - 1) inlineNodes.push(hardBreakType.create());
      });

      return paragraphType.create(null, inlineNodes);
    })
  );
}

export function hasDefaultEmptyDocument(e: Editor): boolean {
  return isDefaultEmptyDocument(e.state.doc);
}

function quoteParagraphsForText(text: string): JSONContent[] {
  return text
    .replace(/\r\n?/g, '\n')
    .split('\n')
    .map((line) => ({
      type: 'paragraph',
      content: line ? [{ type: 'text', text: line }] : undefined
    }));
}

export function buildQuoteContent(blocks: SelectedQuoteBlock[]): JSONContent[] {
  const content: JSONContent[] = [];

  for (const block of blocks) {
    let target = content;

    for (let depth = 0; depth < block.quoteDepth; depth++) {
      let current = target.at(-1);
      if (current?.type !== 'blockquote') {
        current = { type: 'blockquote', content: [] };
        target.push(current);
      }
      current.content ??= [];
      target = current.content;
    }

    target.push(...quoteParagraphsForText(block.text));
  }

  return content;
}
