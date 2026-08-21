import { syntaxTree } from '@codemirror/language';
import type { EditorState } from '@codemirror/state';
import type {
  ComposerFormattingCommand,
  ComposerFormattingState,
  ComposerListIndentDirection,
  ComposerListIndentState
} from './editorTypes';

export type SourceSelection = { anchor: number; head: number };
export type SourceFormattingResult = SourceSelection & { text: string };

type SourceEdit = { from: number; to: number; insert: string };
type SourceSyntaxNode = ReturnType<ReturnType<typeof syntaxTree>['resolveInner']>;

const inlineDelimiters: Partial<Record<ComposerFormattingCommand, [string, string]>> = {
  bold: ['**', '**'],
  italic: ['*', '*'],
  inlineCode: ['`', '`']
};

/** Apply a toolbar command to Markdown source while retaining its logical selection. */
export function applySourceFormatting(
  text: string,
  selection: SourceSelection,
  command: ComposerFormattingCommand
): SourceFormattingResult {
  const delimiter = inlineDelimiters[command];
  if (delimiter) return toggleInline(text, selection, ...delimiter);
  if (command === 'codeBlock') return toggleFence(text, selection);
  if (isLineCommand(command)) return toggleLinePrefix(text, selection, command);
  return { text, ...selection };
}

/** Compute the toolbar state from CodeMirror's parsed Markdown at the primary selection. */
export function getSourceFormattingState(state: EditorState): ComposerFormattingState {
  const activeNames = new Set<string>();
  let node = syntaxTree(state).resolveInner(state.selection.main.head, -1);
  for (;;) {
    activeNames.add(node.name);
    if (!node.parent) break;
    node = node.parent;
  }

  return {
    bold: activeNames.has('StrongEmphasis'),
    italic: activeNames.has('Emphasis'),
    inlineCode: activeNames.has('InlineCode'),
    heading: [...activeNames].some((name) => /^ATXHeading[1-6]$/.test(name)),
    bulletList: activeNames.has('BulletList'),
    orderedList: activeNames.has('OrderedList'),
    blockquote: activeNames.has('Blockquote'),
    codeBlock: activeNames.has('FencedCode') || activeNames.has('CodeBlock')
  };
}

/** Report which structural list-nesting actions apply to the primary selection. */
export function getSourceListIndentState(state: EditorState): ComposerListIndentState {
  const items = selectedListItems(state);
  if (!items) return { canIndent: false, canOutdent: false };

  return {
    canIndent: previousListItem(items[0]!) !== null,
    canOutdent: true
  };
}

/** Structurally indent or outdent selected Markdown list items. */
export function adjustSourceListIndent(
  state: EditorState,
  direction: ComposerListIndentDirection
): SourceFormattingResult | null {
  const items = selectedListItems(state);
  if (!items) return null;

  if (direction === 'outdent' && !parentListItem(items[0]!)) {
    return liftRootListItems(state, items);
  }

  const amount =
    direction === 'indent'
      ? indentationWidthForListItem(state, previousListItem(items[0]!))
      : outdentWidthForListItem(state, items[0]!);
  if (amount <= 0) return null;

  const selection = state.selection.main;
  const edits = indentationEdits(
    state.doc.toString(),
    items[0]!.from,
    items.at(-1)!.to,
    amount,
    direction
  );
  if (edits.length === 0) return null;
  return applySourceEdits(
    state.doc.toString(),
    { anchor: selection.anchor, head: selection.head },
    edits
  );
}

function liftRootListItems(
  state: EditorState,
  items: SourceSyntaxNode[]
): SourceFormattingResult | null {
  const text = state.doc.toString();
  const selection = state.selection.main;
  const edits: SourceEdit[] = [];

  for (const item of items) {
    const markerWidth = indentationWidthForListItem(state, item);
    if (markerWidth <= 0) return null;
    edits.push({ from: item.from, to: item.from + markerWidth, insert: '' });
    edits.push(...indentationEdits(text, item.from, item.to, markerWidth, 'outdent'));
  }

  const previous = previousListItem(items[0]!);
  if (previous && itemsAreOnAdjacentLines(state, previous, items[0]!)) {
    edits.push(listSeparatorEdit(text, state.doc.lineAt(items[0]!.from).from));
  }
  for (let index = 1; index < items.length; index += 1) {
    if (itemsAreOnAdjacentLines(state, items[index - 1]!, items[index]!)) {
      edits.push(listSeparatorEdit(text, state.doc.lineAt(items[index]!.from).from));
    }
  }
  const next = nextListItem(items.at(-1)!);
  if (next && itemsAreOnAdjacentLines(state, items.at(-1)!, next)) {
    edits.push(listSeparatorEdit(text, state.doc.lineAt(next.from).from));
  }

  return applySourceEdits(text, { anchor: selection.anchor, head: selection.head }, edits);
}

function toggleInline(
  text: string,
  selection: SourceSelection,
  opening: string,
  closing: string
): SourceFormattingResult {
  const from = Math.min(selection.anchor, selection.head);
  const to = Math.max(selection.anchor, selection.head);
  const reversed = selection.anchor > selection.head;

  if (from === to) {
    const inserted = opening + closing;
    const cursor = from + opening.length;
    return {
      text: text.slice(0, from) + inserted + text.slice(to),
      anchor: cursor,
      head: cursor
    };
  }

  const isWrapped =
    text.slice(from - opening.length, from) === opening &&
    text.slice(to, to + closing.length) === closing;
  if (isWrapped) {
    const unwrapped =
      text.slice(0, from - opening.length) + text.slice(from, to) + text.slice(to + closing.length);
    const nextFrom = from - opening.length;
    const nextTo = to - opening.length;
    return reversed
      ? { text: unwrapped, anchor: nextTo, head: nextFrom }
      : { text: unwrapped, anchor: nextFrom, head: nextTo };
  }

  const wrapped = text.slice(0, from) + opening + text.slice(from, to) + closing + text.slice(to);
  const nextFrom = from + opening.length;
  const nextTo = to + opening.length;
  return reversed
    ? { text: wrapped, anchor: nextTo, head: nextFrom }
    : { text: wrapped, anchor: nextFrom, head: nextTo };
}

function isLineCommand(
  command: ComposerFormattingCommand
): command is 'heading' | 'bulletList' | 'orderedList' | 'blockquote' {
  return ['heading', 'bulletList', 'orderedList', 'blockquote'].includes(command);
}

function toggleLinePrefix(
  text: string,
  selection: SourceSelection,
  command: Exclude<ComposerFormattingCommand, 'bold' | 'italic' | 'inlineCode' | 'codeBlock'>
): SourceFormattingResult {
  const range = selectedLineRange(text, selection);
  const lines = linesInRange(text, range.from, range.to);
  const nonEmpty = lines.filter((line) => line.text.trim().length > 0);
  const targetLines = nonEmpty.length > 0 ? nonEmpty : lines.slice(0, 1);
  const matcher = prefixMatcher(command);
  const remove = targetLines.every((line) => matcher.test(line.text));
  const edits: SourceEdit[] = [];
  let orderedIndex = 1;

  for (const line of targetLines) {
    const existing = line.text.match(matcher);
    if (remove && existing) {
      edits.push({
        from: line.from,
        to: line.from + existing[0].length,
        insert: existing[1] ?? ''
      });
      continue;
    }

    const indentation = line.text.match(/^\s*/)?.[0] ?? '';
    const insert = prefixFor(command, orderedIndex++);
    edits.push({
      from: line.from,
      to: line.from + (existing?.[0].length ?? indentation.length),
      insert: `${indentation}${insert}`
    });
  }

  return applySourceEdits(text, selection, edits);
}

function toggleFence(text: string, selection: SourceSelection): SourceFormattingResult {
  const from = Math.min(selection.anchor, selection.head);
  const enclosing = enclosingFence(text, from);
  if (enclosing) {
    const replacement = text.slice(enclosing.contentFrom, enclosing.contentTo);
    return {
      text: text.slice(0, enclosing.from) + replacement + text.slice(enclosing.to),
      anchor: enclosing.from,
      head: enclosing.from + replacement.length
    };
  }

  const range = selectedLineRange(text, selection);
  const content = text.slice(range.from, range.to);
  const replacement = `\`\`\`\n${content}\n\`\`\``;
  return {
    text: text.slice(0, range.from) + replacement + text.slice(range.to),
    anchor: range.from + 4,
    head: range.from + 4 + content.length
  };
}

function prefixMatcher(
  command: Exclude<ComposerFormattingCommand, 'bold' | 'italic' | 'inlineCode' | 'codeBlock'>
): RegExp {
  switch (command) {
    case 'heading':
      return /^(\s*)#{1,6}[ \t]+/;
    case 'bulletList':
      return /^(\s*)[-+*][ \t]+/;
    case 'orderedList':
      return /^(\s*)\d{1,9}[.)][ \t]+/;
    case 'blockquote':
      return /^(\s*)>[ \t]?/;
  }
}

function prefixFor(
  command: Exclude<ComposerFormattingCommand, 'bold' | 'italic' | 'inlineCode' | 'codeBlock'>,
  orderedIndex: number
): string {
  switch (command) {
    case 'heading':
      return '## ';
    case 'bulletList':
      return '- ';
    case 'orderedList':
      return `${orderedIndex}. `;
    case 'blockquote':
      return '> ';
  }
}

function selectedLineRange(text: string, selection: SourceSelection): { from: number; to: number } {
  const from = Math.min(selection.anchor, selection.head);
  const rawTo = Math.max(selection.anchor, selection.head);
  const effectiveTo = rawTo > from && text[rawTo - 1] === '\n' ? rawTo - 1 : rawTo;
  const lineFrom = text.lastIndexOf('\n', Math.max(0, from - 1)) + 1;
  const nextBreak = text.indexOf('\n', effectiveTo);
  return { from: lineFrom, to: nextBreak === -1 ? text.length : nextBreak };
}

function linesInRange(text: string, from: number, to: number) {
  const result: { from: number; text: string }[] = [];
  let cursor = from;
  for (const line of text.slice(from, to).split('\n')) {
    result.push({ from: cursor, text: line });
    cursor += line.length + 1;
  }
  return result;
}

function applySourceEdits(
  text: string,
  selection: SourceSelection,
  edits: SourceEdit[]
): SourceFormattingResult {
  const ordered = edits.toSorted((a, b) => a.from - b.from || a.to - b.to);
  let result = '';
  let cursor = 0;
  for (const edit of ordered) {
    result += text.slice(cursor, edit.from) + edit.insert;
    cursor = edit.to;
  }
  result += text.slice(cursor);

  return {
    text: result,
    anchor: mapPosition(selection.anchor, ordered),
    head: mapPosition(selection.head, ordered)
  };
}

function selectedListItems(state: EditorState): SourceSyntaxNode[] | null {
  const selection = state.selection.main;
  const start = listItemAt(state, selection.from, selection.from === state.doc.length ? -1 : 1);
  const endPosition = selection.empty ? selection.head : Math.max(selection.from, selection.to - 1);
  const end = listItemAt(state, endPosition, -1);
  if (!start || !end || !sameNode(start.parent, end.parent)) return null;

  const items: SourceSyntaxNode[] = [];
  for (let node: SourceSyntaxNode | null = start; node; node = node.nextSibling) {
    if (node.name === 'ListItem') items.push(node);
    if (sameNode(node, end)) break;
  }
  return items.length > 0 && sameNode(items.at(-1)!, end) ? items : null;
}

function listItemAt(state: EditorState, position: number, side: -1 | 1): SourceSyntaxNode | null {
  let node: SourceSyntaxNode | null = syntaxTree(state).resolveInner(position, side);
  while (node) {
    if (node.name === 'ListItem') return node;
    node = node.parent;
  }
  return null;
}

function previousListItem(item: SourceSyntaxNode): SourceSyntaxNode | null {
  for (let node = item.prevSibling; node; node = node.prevSibling) {
    if (node.name === 'ListItem') return node;
  }
  return null;
}

function nextListItem(item: SourceSyntaxNode): SourceSyntaxNode | null {
  for (let node = item.nextSibling; node; node = node.nextSibling) {
    if (node.name === 'ListItem') return node;
  }
  return null;
}

function parentListItem(item: SourceSyntaxNode): SourceSyntaxNode | null {
  const parent = item.parent?.parent;
  return parent?.name === 'ListItem' ? parent : null;
}

function sameNode(left: SourceSyntaxNode | null, right: SourceSyntaxNode | null): boolean {
  return (
    left !== null &&
    right !== null &&
    left.name === right.name &&
    left.from === right.from &&
    left.to === right.to
  );
}

function indentationWidthForListItem(state: EditorState, item: SourceSyntaxNode | null): number {
  if (!item) return 0;
  const line = state.doc.lineAt(item.from);
  const marker = line.text.slice(item.from - line.from).match(/^([-+*]|\d{1,9}[.)])([ \t]+)/);
  return marker?.[0].length ?? 0;
}

function outdentWidthForListItem(state: EditorState, item: SourceSyntaxNode): number {
  const parent = parentListItem(item);
  if (!parent) return 0;
  return Math.max(0, listItemIndent(state, item) - listItemIndent(state, parent));
}

function listItemIndent(state: EditorState, item: SourceSyntaxNode): number {
  const line = state.doc.lineAt(item.from);
  return item.from - line.from - blockquotePrefixLength(line.text);
}

function indentationEdits(
  text: string,
  from: number,
  to: number,
  amount: number,
  direction: ComposerListIndentDirection
): SourceEdit[] {
  const edits: SourceEdit[] = [];
  const firstLineStart = text.lastIndexOf('\n', Math.max(0, from - 1)) + 1;
  const finalBreak = text.indexOf('\n', to);
  const rangeEnd = finalBreak === -1 ? text.length : finalBreak;
  let lineStart = firstLineStart;

  while (lineStart <= rangeEnd) {
    const lineEnd = text.indexOf('\n', lineStart);
    const effectiveEnd = lineEnd === -1 || lineEnd > rangeEnd ? rangeEnd : lineEnd;
    const line = text.slice(lineStart, effectiveEnd);
    const prefixLength = blockquotePrefixLength(line);
    const content = line.slice(prefixLength);
    if (content.trim().length > 0) {
      const editFrom = lineStart + prefixLength;
      if (direction === 'indent') {
        edits.push({ from: editFrom, to: editFrom, insert: ' '.repeat(amount) });
      } else {
        const whitespace = content.match(/^[ \t]*/)?.[0].length ?? 0;
        const remove = Math.min(amount, whitespace);
        if (remove > 0) edits.push({ from: editFrom, to: editFrom + remove, insert: '' });
      }
    }
    if (lineEnd === -1 || lineEnd >= rangeEnd) break;
    lineStart = lineEnd + 1;
  }
  return edits;
}

function itemsAreOnAdjacentLines(
  state: EditorState,
  first: SourceSyntaxNode,
  second: SourceSyntaxNode
): boolean {
  return state.doc.lineAt(second.from).number === state.doc.lineAt(first.to).number + 1;
}

function listSeparatorEdit(text: string, lineStart: number): SourceEdit {
  const lineEnd = text.indexOf('\n', lineStart);
  const line = text.slice(lineStart, lineEnd === -1 ? text.length : lineEnd);
  const quotePrefix = line.slice(0, blockquotePrefixLength(line)).trimEnd();
  return { from: lineStart, to: lineStart, insert: quotePrefix ? `${quotePrefix}\n` : '\n' };
}

function blockquotePrefixLength(line: string): number {
  let length = 0;
  while (length < line.length) {
    const match = line.slice(length).match(/^[ \t]{0,3}>[ \t]?/);
    if (!match) break;
    length += match[0].length;
  }
  return length;
}

function mapPosition(position: number, edits: SourceEdit[]): number {
  let delta = 0;
  for (const edit of edits) {
    if (position < edit.from) break;
    if (position <= edit.to) return edit.from + delta + edit.insert.length;
    delta += edit.insert.length - (edit.to - edit.from);
  }
  return position + delta;
}

function enclosingFence(text: string, position: number) {
  const lines = text.split('\n');
  let offset = 0;
  let opening: { from: number; end: number; marker: string } | null = null;

  for (const line of lines) {
    const from = offset;
    const end = from + line.length;
    const marker = line.match(/^ {0,3}(`{3,}|~{3,})/)?.[1];
    if (marker) {
      if (!opening) {
        opening = { from, end, marker };
      } else if (marker[0] === opening.marker[0] && marker.length >= opening.marker.length) {
        if (position >= opening.from && position <= end) {
          return {
            from: opening.from,
            to: end,
            contentFrom: Math.min(text.length, opening.end + 1),
            contentTo: Math.max(opening.end + 1, from - 1)
          };
        }
        opening = null;
      }
    }
    offset = end + 1;
  }
  return null;
}
