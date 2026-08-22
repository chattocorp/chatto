import { syntaxTree } from '@codemirror/language';
import type { EditorState } from '@codemirror/state';
import type { ComposerFormattingCommand, ComposerFormattingState } from './editorTypes';

export type SourceSelection = { anchor: number; head: number };
export type SourceFormattingResult = SourceSelection & { text: string };

type SourceEdit = { from: number; to: number; insert: string };

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
  const ordered = edits.toSorted((a, b) => a.from - b.from);
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
