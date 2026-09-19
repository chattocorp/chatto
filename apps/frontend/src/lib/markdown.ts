import MarkdownIt from 'markdown-it';
import type StateInline from 'markdown-it/lib/rules_inline/state_inline.mjs';
import type StateCore from 'markdown-it/lib/rules_core/state_core.mjs';
import type StateBlock from 'markdown-it/lib/rules_block/state_block.mjs';
import type Token from 'markdown-it/lib/token.mjs';
import { isSpace } from 'markdown-it/lib/common/utils.mjs';
import tlds from 'tlds';
import { classifyMessageBodyChatLink } from '$lib/messageLinks';

type CodeHighlightingModule = typeof import('$lib/codeHighlighting');

/**
 * Disabled markdown-it rules - we only allow a subset of markdown syntax.
 */
const DISABLED_RULES = [
  // Block-level
  'lheading',
  'hr',
  'reference',
  // Inline
  'image',
  'html_inline',
  // Backslash escapes turn `\_` into a literal `_`, which eats the arms of
  // common kaomoji like ¯\_(ツ)_/¯. Chat users type literal backslashes far
  // more often than they need CommonMark escapes; code spans still work for
  // escaping markdown chars when needed.
  'escape'
] as const;

const ALPHANUMERIC = /[a-zA-Z0-9]/;

/**
 * Spoiler syntax (`||text||`) limits. Spoilers are presentation-only and never
 * a security boundary, but adversarial bodies must not be able to create huge
 * numbers of spoiler regions.
 */
const SPOILER_MARKER = 0x7c; // '|'
const MAX_SPOILERS_PER_MESSAGE = 100;
const spoilerCountKey = Symbol('spoilerCount');
const MAX_TABLE_COLUMNS = 64;
const MAX_TABLE_ROWS = 256;
const MAX_TABLE_CELLS = 4_096;
const MAX_TABLE_CELLS_PER_MESSAGE = 8_192;
const tableCellCountKey = Symbol('tableCellCount');

type MarkdownBlockRule = (
  state: StateBlock,
  startLine: number,
  endLine: number,
  silent: boolean
) => boolean;

function getBlockLine(state: StateBlock, line: number): string {
  const start = state.bMarks[line] + state.tShift[line];
  return state.src.slice(start, state.eMarks[line]);
}

/**
 * Marks every pipe character that belongs to a paired spoiler delimiter run
 * (`||...||`) in one line of text. Shared by the forked GFM table rule and its
 * cell splitter so spoiler delimiters inside cells stay cell content instead
 * of being treated as column separators.
 *
 * Pairing mirrors the inline tokenizer's whole-run delimiter semantics:
 * maximal runs of two or more pipes, openers followed by non-whitespace,
 * closers preceded by non-whitespace, nearest-opener-first matching, and no
 * empty spans.
 */
function markSpoilerDelimiterPipes(text: string): boolean[] {
  const masked = new Array<boolean>(text.length).fill(false);
  type PipeRun = { start: number; end: number; canOpen: boolean; canClose: boolean };
  const runs: PipeRun[] = [];

  let index = 0;
  while (index < text.length) {
    if (text.charCodeAt(index) !== SPOILER_MARKER) { index++; continue; }
    let end = index;
    while (end < text.length && text.charCodeAt(end) === SPOILER_MARKER) end++;
    if (end - index >= 2) {
      const previous = index > 0 ? text[index - 1] : '';
      const next = end < text.length ? text[end] : '';
      runs.push({
        start: index,
        end,
        canOpen: next !== '' && !/\s/.test(next),
        canClose: previous !== '' && !/\s/.test(previous)
      });
    }
    index = end;
  }

  const openerStack: PipeRun[] = [];
  for (const run of runs) {
    const top = openerStack[openerStack.length - 1];
    if (run.canClose && top && top.end < run.start) {
      // Accepted pair: mask every pipe of both delimiter runs.
      for (let i = top.start; i < top.end; i++) masked[i] = true;
      for (let i = run.start; i < run.end; i++) masked[i] = true;
      openerStack.pop();
    } else if (run.canOpen) {
      openerStack.push(run);
    }
  }
  return masked;
}

/**
 * Splits one table row into cells. Same contract as markdown-it's internal
 * `escapedSplit`, plus: pipe characters belonging to paired spoiler delimiter
 * runs are kept verbatim as cell content rather than acting as separators.
 */
function splitTableRowWithSpoilers(row: string): string[] {
  const masked = markSpoilerDelimiterPipes(row);
  const result: string[] = [];
  let lastPosition = 0;
  let current = '';
  let escaped = false;

  for (let position = 0; position < row.length; position++) {
    const char = row[position];
    if (char === '|' && !escaped && !masked[position]) {
      result.push(current + row.slice(lastPosition, position));
      current = '';
      lastPosition = position + 1;
    } else if (char === '|' && escaped) {
      current += row.slice(lastPosition, position - 1);
      lastPosition = position;
    }

    escaped = char === '\\';
  }
  result.push(current + row.slice(lastPosition));

  return result;
}

function tableColumnCount(state: StateBlock, startLine: number, endLine: number): number | null {
  if (startLine + 2 > endLine) return null;

  const delimiterLine = getBlockLine(state, startLine + 1);
  if (!/^[|:\-\t ]+$/.test(delimiterLine)) return null;

  const delimiters = delimiterLine.split('|');
  if (delimiters[0]?.trim() === '') delimiters.shift();
  if (delimiters.at(-1)?.trim() === '') delimiters.pop();
  if (delimiters.length === 0 || delimiters.some((cell) => !/^:?-+:?$/.test(cell.trim()))) {
    return null;
  }

  const headerLine = getBlockLine(state, startLine).trim();
  if (!headerLine.includes('|')) return null;

  const headers = splitTableRowWithSpoilers(headerLine);
  if (headers[0] === '') headers.shift();
  if (headers.at(-1) === '') headers.pop();

  return headers.length === delimiters.length ? headers.length : null;
}

function countTableCells(
  state: StateBlock,
  startLine: number,
  endLine: number,
  columnCount: number
): number {
  let rowCount = 1;
  const terminatorRules = state.md.block.ruler.getRules('blockquote');

  for (let line = startLine + 2; line < endLine; line++) {
    if (state.sCount[line] < state.blkIndent) break;
    if (state.sCount[line] - state.blkIndent >= 4) break;
    if (terminatorRules.some((rule) => rule(state, line, endLine, true))) break;
    if (!getBlockLine(state, line).trim()) break;

    rowCount++;
    if (rowCount > MAX_TABLE_ROWS || rowCount * columnCount > MAX_TABLE_CELLS) {
      return MAX_TABLE_CELLS + 1;
    }
  }

  return rowCount * columnCount;
}

function boundedTableRule(tableRule: MarkdownBlockRule): MarkdownBlockRule {
  return (state, startLine, endLine, silent) => {
    const columnCount = tableColumnCount(state, startLine, endLine);
    if (columnCount === null) return false;
    if (columnCount > MAX_TABLE_COLUMNS) return false;

    const cellCount = countTableCells(state, startLine, endLine, columnCount);
    const renderedCellCount = (state.env[tableCellCountKey] as number | undefined) ?? 0;
    if (
      cellCount > MAX_TABLE_CELLS ||
      renderedCellCount + cellCount > MAX_TABLE_CELLS_PER_MESSAGE
    ) {
      return false;
    }

    const parsed = tableRule(state, startLine, endLine, silent);
    if (parsed && !silent) state.env[tableCellCountKey] = renderedCellCount + cellCount;
    return parsed;
  };
}

// Limit on empty autocompleted cells from upstream markdown-it's GFM table
// rule; kept identical while the fork below adds spoiler-aware cell splits.
const MAX_AUTOCOMPLETED_CELLS = 0x10000;

/**
 * Fork of markdown-it's built-in GFM table block rule.
 *
 * Identical to upstream except that row splitting goes through
 * `splitTableRowWithSpoilers`, so pipes inside paired `||...||` spoiler
 * delimiters remain part of a cell instead of acting as column separators.
 * Must be re-checked when bumping markdown-it versions.
 */
function chattoTableRule(
  state: StateBlock,
  startLine: number,
  endLine: number,
  silent: boolean
): boolean {
  if (startLine + 2 > endLine) return false;

  let nextLine = startLine + 1;
  if (state.sCount[nextLine] < state.blkIndent) return false;
  if (state.sCount[nextLine] - state.blkIndent >= 4) return false;

  let pos = state.bMarks[nextLine] + state.tShift[nextLine];
  if (pos >= state.eMarks[nextLine]) return false;

  const firstCh = state.src.charCodeAt(pos++);
  if (firstCh !== 0x7c/* | */ && firstCh !== 0x2d/* - */ && firstCh !== 0x3a/* : */) return false;
  if (pos >= state.eMarks[nextLine]) return false;

  const secondCh = state.src.charCodeAt(pos++);
  if (
    secondCh !== 0x7c/* | */ &&
    secondCh !== 0x2d/* - */ &&
    secondCh !== 0x3a/* : */ &&
    !isSpace(secondCh)
  ) {
    return false;
  }
  if (firstCh === 0x2d/* - */ && isSpace(secondCh)) return false;

  while (pos < state.eMarks[nextLine]) {
    const ch = state.src.charCodeAt(pos);
    if (ch !== 0x7c/* | */ && ch !== 0x2d/* - */ && ch !== 0x3a/* : */ && !isSpace(ch)) return false;
    pos++;
  }

  let lineText = getBlockLine(state, startLine + 1);
  let columns = lineText.split('|');
  const aligns: string[] = [];
  for (let i = 0; i < columns.length; i++) {
    const t = columns[i].trim();
    if (!t) {
      // allow empty columns before and after table, but not in between columns
      if (i === 0 || i === columns.length - 1) continue;
      return false;
    }
    if (!/^:?-+:?$/.test(t)) return false;
    if (t.charCodeAt(t.length - 1) === 0x3a/* : */) {
      aligns.push(t.charCodeAt(0) === 0x3a/* : */ ? 'center' : 'right');
    } else if (t.charCodeAt(0) === 0x3a/* : */) {
      aligns.push('left');
    } else {
      aligns.push('');
    }
  }

  lineText = getBlockLine(state, startLine).trim();
  if (!lineText.includes('|')) return false;
  if (state.sCount[startLine] - state.blkIndent >= 4) return false;
  columns = splitTableRowWithSpoilers(lineText);
  if (columns.length && columns[0] === '') columns.shift();
  if (columns.length && columns[columns.length - 1] === '') columns.pop();

  const columnCount = columns.length;
  if (columnCount === 0 || columnCount !== aligns.length) return false;

  if (silent) return true;

  const oldParentType = state.parentType;
  state.parentType = 'table';

  const terminatorRules = state.md.block.ruler.getRules('blockquote');

  const tokenTo = state.push('table_open', 'table', 1);
  const tableLines = [startLine, 0];
  tokenTo.map = tableLines;

  const tokenTho = state.push('thead_open', 'thead', 1);
  tokenTho.map = [startLine, startLine + 1];

  const tokenHtro = state.push('tr_open', 'tr', 1);
  tokenHtro.map = [startLine, startLine + 1];

  for (let i = 0; i < columns.length; i++) {
    const tokenHo = state.push('th_open', 'th', 1);
    if (aligns[i]) tokenHo.attrs = [['style', 'text-align:' + aligns[i]]];

    const tokenIl = state.push('inline', '', 0);
    tokenIl.content = columns[i].trim();
    tokenIl.children = [];

    state.push('th_close', 'th', -1);
  }

  state.push('tr_close', 'tr', -1);
  state.push('thead_close', 'thead', -1);

  let tbodyLines: [number, number] | undefined;
  let autocompletedCells = 0;

  for (nextLine = startLine + 2; nextLine < endLine; nextLine++) {
    if (state.sCount[nextLine] < state.blkIndent) break;

    let terminate = false;
    for (let i = 0; i < terminatorRules.length; i++) {
      if (terminatorRules[i](state, nextLine, endLine, true)) {
        terminate = true;
        break;
      }
    }
    if (terminate) break;

    lineText = getBlockLine(state, nextLine).trim();
    if (!lineText) break;
    if (state.sCount[nextLine] - state.blkIndent >= 4) break;

    columns = splitTableRowWithSpoilers(lineText);
    if (columns.length && columns[0] === '') columns.shift();
    if (columns.length && columns[columns.length - 1] === '') columns.pop();

    autocompletedCells += columnCount - columns.length;
    if (autocompletedCells > MAX_AUTOCOMPLETED_CELLS) break;

    if (nextLine === startLine + 2) {
      const tokenTbo = state.push('tbody_open', 'tbody', 1);
      tokenTbo.map = tbodyLines = [startLine + 2, 0];
    }

    const tokenTro = state.push('tr_open', 'tr', 1);
    tokenTro.map = [nextLine, nextLine + 1];

    for (let i = 0; i < columnCount; i++) {
      const tokenTd = state.push('td_open', 'td', 1);
      if (aligns[i]) tokenTd.attrs = [['style', 'text-align:' + aligns[i]]];

      const tokenIl = state.push('inline', '', 0);
      tokenIl.content = columns[i] ? columns[i].trim() : '';
      tokenIl.children = [];

      state.push('td_close', 'td', -1);
    }
    state.push('tr_close', 'tr', -1);
  }

  if (tbodyLines) {
    state.push('tbody_close', 'tbody', -1);
    tbodyLines[1] = nextLine;
  }

  state.push('table_close', 'table', -1);
  tableLines[1] = nextLine;

  state.parentType = oldParentType;
  state.line = nextLine;
  return true;
}

/**
 * Inline rule that consumes `*` or `_` marker runs as literal text when they
 * are not at a word boundary. A word boundary requires exactly one side of
 * the run to be alphanumeric. This neuters intraword emphasis like
 * `foo*bar*baz` and punctuation-flanked markers like `_(ツ)_`, while
 * preserving normal `*italic*`, `_italic_`, and `**bold**`.
 */
function wordBoundaryEmphasis(state: StateInline, silent: boolean): boolean {
  const start = state.pos;
  const marker = state.src.charCodeAt(start);
  if (marker !== 0x2a /* * */ && marker !== 0x5f /* _ */) return false;

  let runEnd = start + 1;
  while (runEnd < state.posMax && state.src.charCodeAt(runEnd) === marker) {
    runEnd++;
  }
  const runLength = runEnd - start;

  const before = start > 0 ? state.src[start - 1] : '';
  const after = runEnd < state.src.length ? state.src[runEnd] : '';
  const beforeAlnum = ALPHANUMERIC.test(before);
  const afterAlnum = ALPHANUMERIC.test(after);

  // Single-marker intraword runs are definitely literal (`snake_case`,
  // `foo*bar*baz`). Double-marker runs are still allowed so bold can end next
  // to a following word (`**bold**text`).
  const intraword = runLength === 1 && beforeAlnum && afterAlnum;
  // Kaomoji-like: punctuation on both sides AND neither direction crosses an
  // alphanumeric before hitting a same-marker run or the input boundary. The
  // bidirectional check distinguishes a true kaomoji marker (e.g. the trailing
  // `_` in `_(ツ)_/¯` — only punctuation back to the opener and only
  // punctuation forward to end of input) from a closer of a real emphasis
  // run that happens to be followed by punctuation/another emphasis (e.g.
  // the closing `**` in `**foo:** **bar**` — alnum `o` is right behind it).
  let kaomojiLike = false;
  if (!beforeAlnum && !afterAlnum) {
    let forwardOK = true;
    for (let i = runEnd; i < state.posMax; i++) {
      if (state.src.charCodeAt(i) === marker) break;
      if (ALPHANUMERIC.test(state.src[i])) {
        forwardOK = false;
        break;
      }
    }
    if (forwardOK) {
      kaomojiLike = true;
      for (let i = start - 1; i >= 0; i--) {
        if (state.src.charCodeAt(i) === marker) break;
        if (ALPHANUMERIC.test(state.src[i])) {
          kaomojiLike = false;
          break;
        }
      }
    }
  }
  if (intraword || kaomojiLike) {
    if (!silent) state.pending += state.src.slice(start, runEnd);
    state.pos = runEnd;
    return true;
  }

  return false;
}

/**
 * Copy of markdown-it's default inline text rule with one change: `|`
 * (0x7c) terminates plain-text runs so the spoiler tokenizer can claim pipe
 * runs. Without this, the default rule swallows whole runs of pipes before
 * any extension rule can see them.
 */
function isTextTerminator(ch: number): boolean {
  switch (ch) {
    case 0x0a/* \n */:
    case 0x21/* ! */:
    case 0x23/* # */:
    case 0x24/* $ */:
    case 0x25/* % */:
    case 0x26/* & */:
    case 0x2a/* * */:
    case 0x2b/* + */:
    case 0x2d/* - */:
    case 0x3a/* : */:
    case 0x3c/* < */:
    case 0x3d/* = */:
    case 0x3e/* > */:
    case 0x40/* @ */:
    case 0x5b/* [ */:
    case 0x5c/* \ */:
    case 0x5d/* ] */:
    case 0x5e/* ^ */:
    case 0x5f/* _ */:
    case 0x60/* ` */:
    case 0x7b/* { */:
    case 0x7c/* | */:
    case 0x7d/* } */:
    case 0x7e/* ~ */:
      return true;
    default:
      return false;
  }
}

function chattoText(state: StateInline, silent: boolean): boolean {
  let pos = state.pos;

  while (pos < state.posMax && !isTextTerminator(state.src.charCodeAt(pos))) {
    pos++;
  }

  if (pos === state.pos) return false;

  if (!silent) state.pending += state.src.slice(state.pos, pos);

  state.pos = pos;

  return true;
}

/**
 * Inline rule that consumes maximal runs of `|` characters. Each run of two or
 * more pipes registers one candidate spoiler delimiter in `state.delimiters`
 * (carrying absolute source offsets alongside the standard fields); single
 * pipes stay literal text. The resolution pass below validates and converts
 * paired runs once `balance_pairs` has linked them up.
 *
 * Delimiter rules, per FDR-032:
 * - an opener's run must be followed by a non-whitespace character;
 * - a closer's run must be preceded by a non-whitespace character;
 * - empty and unmatched runs stay literal.
 */
function spoilerTokenizer(state: StateInline, silent: boolean): boolean {
  const start = state.pos;
  if (state.src.charCodeAt(start) !== SPOILER_MARKER) return false;
  let end = start;
  while (end < state.posMax && state.src.charCodeAt(end) === SPOILER_MARKER) end++;
  const runLength = end - start;

  const previous = start > 0 ? state.src[start - 1] : '';
  const next = end < state.posMax ? state.src[end] : '';
  const spaceBefore = previous === '' || /\s/.test(previous);
  const spaceAfter = next === '' || /\s/.test(next);

  if (!silent) {
    // One token per whole run: the resolver converts these into
    // spoiler_open/spoiler_close pairs (or leaves them literal).
    state.push('text', '', 0).content = state.src.slice(start, end);
    if (runLength >= 2) {
      state.delimiters.push({
        marker: SPOILER_MARKER,
        // Disables the emphasis "rule of 3" length checks in balance_pairs.
        length: 0,
        token: state.tokens.length - 1,
        end: -1,
        open: !spaceAfter,
        close: !spaceBefore,
        spoilerSrcStart: start,
        spoilerSrcEnd: end
      } as any);
    }
  }

  state.pos = end;
  return true;
}

/**
 * Resolution pass for spoiler delimiters, mirroring the emphasis/strikethrough
 * post-processing contract. `balance_pairs` has already paired up same-marker
 * delimiters nearest-opener-first and recorded the partner index in
 * `delimiters[i].end`; this pass validates each pipe pair against FDR-032
 * semantics and converts accepted pairs into spoiler_open/spoiler_close
 * tokens so enclosed inline Markdown keeps rendering normally.
 *
 * A pair is rejected (both markers revert to literal text) when its content
 * is empty, when it would nest one spoiler inside another one, or when the
 * message already hit the spoiler limit. Spans are validated innermost-first,
 * so in `||a ||b|| c||` only `b` stays spoiled; rejected outer markers stay
 * literal.
 *
 * Crossing paragraph/block boundaries is impossible by construction because
 * every inline chunk carries its own delimiter set.
 */
type SpoilerDelimiter = {
  open: boolean;
  close: boolean;
  end: number;
  token: number;
  spoilerSrcStart: number;
  spoilerSrcEnd: number;
};

function resolveSpoilerDelimiters(
  state: StateInline,
  delimiters: SpoilerDelimiter[]
): void {
  const candidatePairs: Array<[SpoilerDelimiter, SpoilerDelimiter]> = [];
  for (const delimiter of delimiters) {
    if (delimiter.marker !== SPOILER_MARKER) continue;
    if (typeof delimiter.spoilerSrcStart !== 'number') continue;
    // balance_pairs marks a matched OPENER by setting its `end` to the
    // closer's index in this array.
    if (delimiter.end < 0) continue;
    const closer = delimiters[delimiter.end];
    if (closer?.marker === SPOILER_MARKER) candidatePairs.push([delimiter, closer]);
    delimiter.end = -1; // consume, in case resolve runs again for this array
  }
  if (candidatePairs.length === 0) return;

  // Innermost pairs first: shorter spans sorted before ones they could contain.
  candidatePairs.sort(
    ([openA, closeA], [openB, closeB]) =>
      closeA.spoilerSrcStart - openA.spoilerSrcStart -
      (closeB.spoilerSrcStart - openB.spoilerSrcStart)
  );

  const acceptedSpans: Array<[number, number]> = [];
  let converted = 0;
  const previousSpoilers = (state.env[spoilerCountKey] as number | undefined) ?? 0;

  for (const [opener, closer] of candidatePairs) {
    if (closer.spoilerSrcStart <= opener.spoilerSrcEnd) continue; // empty or overlapping content
    const spanStart = opener.spoilerSrcStart;
    const spanEnd = closer.spoilerSrcEnd;
    // Forbid nesting: a previously accepted spoiler must not lie inside.
    if (acceptedSpans.some(([start, end]) => spanStart <= start && end <= spanEnd)) continue;
    if (previousSpoilers + converted >= MAX_SPOILERS_PER_MESSAGE) break;

    acceptedSpans.push([spanStart, spanEnd]);
    converted++;

    const openToken = state.tokens[opener.token];
    const closeToken = state.tokens[closer.token];
    openToken.type = 'spoiler_open';
    openToken.tag = 'span';
    openToken.content = '';
    openToken.markup = '||';
    openToken.nesting = 1;
    closeToken.type = 'spoiler_close';
    closeToken.tag = 'span';
    closeToken.content = '';
    closeToken.markup = '||';
    closeToken.nesting = -1;
  }

  if (converted > 0) {
    state.env[spoilerCountKey] = previousSpoilers + converted;
  }
}

function spoilerResolve(state: StateInline): void {
  resolveSpoilerDelimiters(state, state.delimiters as SpoilerDelimiter[]);
  const metas = (state as StateInline & { tokens_meta?: Array<{ delimiters?: any } | null> }).tokens_meta;
  if (metas) {
    for (const meta of metas) {
      if (meta?.delimiters) {
        resolveSpoilerDelimiters(state, meta.delimiters as SpoilerDelimiter[]);
      }
    }
  }
}

let md: MarkdownIt | null = null;
let codeHighlighting: CodeHighlightingModule | null = null;

type LowlightText = {
  type: 'text';
  value: string;
};

type LowlightElement = {
  type: 'element';
  tagName: string;
  properties?: Record<string, unknown>;
  children?: LowlightNode[];
};

type LowlightNode =
  | LowlightText
  | LowlightElement
  | {
      type: string;
      children?: LowlightNode[];
    };

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function escapeAttribute(value: string): string {
  return escapeHtml(value).replaceAll("'", '&#39;');
}

function renderClassName(value: unknown): string | null {
  if (Array.isArray(value)) {
    const classes = value.filter((item): item is string => typeof item === 'string');
    return classes.length > 0 ? classes.join(' ') : null;
  }

  return typeof value === 'string' && value.length > 0 ? value : null;
}

function renderElementOpen(node: LowlightElement): string {
  const className = renderClassName(node.properties?.className);
  const classAttribute = className ? ` class="${escapeAttribute(className)}"` : '';
  return `<${node.tagName}${classAttribute}>`;
}

function isLowlightText(node: LowlightNode): node is LowlightText {
  return node.type === 'text';
}

function isLowlightElement(node: LowlightNode): node is LowlightElement {
  return node.type === 'element';
}

function renderLowlightLines(nodes: LowlightNode[]): string[] {
  const lines = [''];

  function append(value: string) {
    lines[lines.length - 1] += value;
  }

  function renderNode(node: LowlightNode, activeOpen: string, activeClose: string) {
    if (isLowlightText(node)) {
      const parts = node.value.replaceAll('\t', '    ').split('\n');

      for (let i = 0; i < parts.length; i++) {
        if (i > 0) {
          append(activeClose);
          lines.push(activeOpen);
        }
        append(escapeHtml(parts[i]));
      }
      return;
    }

    if (isLowlightElement(node)) {
      const open = renderElementOpen(node);
      const close = `</${node.tagName}>`;
      append(open);

      for (const child of node.children ?? []) {
        renderNode(child, `${activeOpen}${open}`, `${close}${activeClose}`);
      }

      append(close);
      return;
    }

    for (const child of 'children' in node ? (node.children ?? []) : []) {
      renderNode(child, activeOpen, activeClose);
    }
  }

  for (const node of nodes) {
    renderNode(node, '', '');
  }

  return lines;
}

function renderPlainCodeLines(code: string): string[] {
  return code.replaceAll('\t', '    ').split('\n').map(escapeHtml);
}

function renderCodeFence(code: string, rawLanguage: string): string {
  const displayLanguage = normalizeCodeLanguage(rawLanguage);
  const resolvedLanguage = codeHighlighting?.resolveCodeLanguage(displayLanguage);
  const displayCode = code.replace(/\r?\n$/, '');
  const lines =
    resolvedLanguage && codeHighlighting?.lowlight.registered(displayLanguage)
      ? renderLowlightLines(
          (
            codeHighlighting.lowlight.highlight(displayLanguage, displayCode) as {
              children: LowlightNode[];
            }
          ).children
        )
      : resolvedLanguage && codeHighlighting?.lowlight.registered(resolvedLanguage)
        ? renderLowlightLines(
            (
              codeHighlighting.lowlight.highlight(resolvedLanguage, displayCode) as {
                children: LowlightNode[];
              }
            ).children
          )
        : renderPlainCodeLines(displayCode);
  const lineHtml = lines.map((line) => `<span class="line">${line}</span>`).join('');

  return `<pre class="hljs" data-language="${escapeAttribute(displayLanguage)}"><code class="language-${escapeAttribute(displayLanguage)}">${lineHtml}</code></pre>`;
}

function normalizeCodeLanguage(language: string | null | undefined): string {
  const token = language
    ?.trim()
    .toLowerCase()
    .match(/[a-z0-9+#_.-]+/)?.[0];
  return token || 'text';
}

function extractFenceLanguages(markdown: string): string[] {
  const languages = new Set<string>();
  const fencePattern = /^[ \t]*(```|~~~)[ \t]*([^\s`~]*)/gm;
  let match: RegExpExecArray | null;

  while ((match = fencePattern.exec(markdown))) {
    languages.add(normalizeCodeLanguage(match[2]));
  }

  return [...languages];
}

function isLineBreakToken(token: Token): boolean {
  return token.type === 'softbreak' || token.type === 'hardbreak';
}

function isWhitespaceOnlyInlineSegment(tokens: Token[]): boolean {
  return tokens.every((token) => {
    if (token.type === 'text' || token.type === 'text_special') {
      return token.content.trim().length === 0;
    }
    return token.type.endsWith('_open') || token.type.endsWith('_close');
  });
}

function lineAfterBreakIsWhitespaceOnly(tokens: Token[], idx: number): boolean {
  let lineEnd = idx + 1;
  while (lineEnd < tokens.length && !isLineBreakToken(tokens[lineEnd])) lineEnd++;
  return isWhitespaceOnlyInlineSegment(tokens.slice(idx + 1, lineEnd));
}

function renderChatLineBreak(tokens: Token[], idx: number): string {
  return lineAfterBreakIsWhitespaceOnly(tokens, idx) ? '' : '<br>\n';
}

function renderParagraphOpen(tokens: Token[], idx: number): string {
  return tokens[idx].hidden ? '<span class="list-item-content">' : '<p>';
}

function renderParagraphClose(tokens: Token[], idx: number): string {
  return tokens[idx].hidden ? '</span>\n' : '</p>\n';
}

function normalizeInlineNonBreakingSpaces(state: StateCore): void {
  for (let i = 0; i < state.tokens.length; i++) {
    const token = state.tokens[i];
    if (token.type !== 'inline') continue;
    for (const child of token.children ?? []) {
      if (child.type === 'text' || child.type === 'text_special') {
        child.content = child.content.replaceAll('\u00A0', ' ');
      }
    }
    if (isWhitespaceOnlyInlineSegment(token.children ?? [])) {
      if (state.tokens[i - 1]?.type === 'paragraph_open') state.tokens[i - 1].hidden = true;
      if (state.tokens[i + 1]?.type === 'paragraph_close') state.tokens[i + 1].hidden = true;
    }
  }
}

async function ensureFenceLanguagesLoaded(languages: string[]): Promise<void> {
  if (languages.length === 0) return;

  codeHighlighting ??= await import('$lib/codeHighlighting');
  await codeHighlighting.ensureCodeLanguagesLoaded(languages);
}

/**
 * Initialize the markdown-it instance.
 * Called once on first render.
 */
function initialize(): void {
  if (md) return;

  md = new MarkdownIt({
    html: false, // Disable HTML tags in source
    linkify: true, // Auto-convert URLs to links
    breaks: true, // Convert \n to <br>
    highlight: renderCodeFence
  });

  // Update linkify-it's TLD list so bare-domain URLs with newer TLDs
  // (.dev, .app, .io, etc.) are auto-linked
  md.linkify.tlds(tlds);

  md.block.ruler.at('table', boundedTableRule(chattoTableRule), {
    alt: ['paragraph', 'reference']
  });

  // Disable unwanted syntax - only keep what we explicitly want
  md.disable([...DISABLED_RULES]);

  // Restrict `*` and `_` emphasis to word boundaries. Prevents intraword
  // emphasis (e.g. `snake_case`, `foo*bar*baz`) and emphasis between
  // punctuation (e.g. the underscores in `¯\_(ツ)_/¯`) from being parsed
  // as italics. Inserted before the `emphasis` rule so non-boundary marker
  // runs are consumed as literal text.
  md.inline.ruler.before('emphasis', 'word_boundary_emphasis', wordBoundaryEmphasis);

  md.inline.ruler.at('text', chattoText);
  md.inline.ruler.before('backticks', 'chatto_spoiler', spoilerTokenizer);

  // Pair up spoiler delimiter runs after other inline constructs balanced.
  md.inline.ruler2.after('balance_pairs', 'chatto_spoiler_resolve', spoilerResolve);

  // CommonMark decodes entities in prose but leaves them literal in code. Turn
  // decoded NBSPs into collapsible spaces only in ordinary inline text so long
  // `&nbsp;` runs cannot create giant blank message rows without corrupting
  // code samples that intentionally contain the entity source.
  md.core.ruler.after('inline', 'normalize_non_breaking_spaces', normalizeInlineNonBreakingSpaces);
  md.renderer.rules.softbreak = renderChatLineBreak;
  md.renderer.rules.hardbreak = renderChatLineBreak;
  md.renderer.rules.table_open = () => '<div class="table-scroll" tabindex="0"><table>\n';
  md.renderer.rules.table_close = () => '</table></div>\n';
  // Markdown-it hides paragraph tags in tight lists. Keep their inline content
  // grouped so ordered-list grid markers do not turn each inline element into
  // a separate grid row.
  md.renderer.rules.paragraph_open = renderParagraphOpen;
  md.renderer.rules.paragraph_close = renderParagraphClose;

  // Customize link rendering for security
  const defaultLinkRender =
    md.renderer.rules.link_open ||
    function (tokens, idx, options, _env, self) {
      return self.renderToken(tokens, idx, options);
    };

  md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
    const token = tokens[idx];
    const hrefIndex = token.attrIndex('href');
    let allowedSameTabChatLink = false;

    if (hrefIndex >= 0) {
      const href = token.attrs![hrefIndex][1];

      // Only allow http and https URLs
      if (!href.startsWith('http://') && !href.startsWith('https://')) {
        // Replace dangerous URLs with empty href
        token.attrs![hrefIndex][1] = '#';
      } else {
        allowedSameTabChatLink = classifyMessageBodyChatLink(href) !== null;
      }
    }

    // External and non-allow-listed links open out-of-band. Known same-origin
    // chat routes intentionally keep normal same-tab navigation semantics.
    if (!allowedSameTabChatLink) {
      token.attrSet('target', '_blank');
      token.attrSet('rel', 'noopener noreferrer');
    }

    return defaultLinkRender(tokens, idx, options, env, self);
  };

  md.renderer.rules.spoiler_open = () => '<span class="spoiler" data-spoiler>';
  md.renderer.rules.spoiler_close = () => '</span>';
}

/**
 * Renders inline formatting and safe links on one line, without block markup.
 */
export function renderInlineMarkdown(body: string): string {
  initialize();
  return md!.renderInline(body.replace(/[\r\n]+/g, ' '));
}

/**
 * Renders markdown to HTML.
 */
export async function renderMarkdown(body: string): Promise<string> {
  try {
    await ensureFenceLanguagesLoaded(extractFenceLanguages(body));
    initialize();

    return md!.render(body);
  } catch (err) {
    console.error('[Markdown] renderMarkdown failed:', err, { bodyLength: body.length });
    throw err;
  }
}
