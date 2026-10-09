import { EditorState } from '@codemirror/state';
import { markdown } from '@codemirror/lang-markdown';
import { describe, expect, it } from 'vitest';
import { applySourceFormatting, getSourceFormattingState } from './sourceFormatting';

describe('applySourceFormatting', () => {
  it('inserts a unique footnote after selected text and focuses its definition', () => {
    const source = 'Hello[^1].\n\n[^1]: Existing note.';
    const result = applySourceFormatting(source, { anchor: 0, head: 5 }, 'footnote');
    expect(result.text).toBe('Hello[^2][^1].\n\n[^1]: Existing note.\n\n[^2]: ');
    expect(result.anchor).toBe(result.text.length);
    expect(result.head).toBe(result.anchor);
  });
  it('wraps and unwraps inline selections', () => {
    const wrapped = applySourceFormatting('hello world', { anchor: 0, head: 5 }, 'bold');
    expect(wrapped).toEqual({ text: '**hello** world', anchor: 2, head: 7 });
    expect(applySourceFormatting(wrapped.text, wrapped, 'bold')).toEqual({
      text: 'hello world',
      anchor: 0,
      head: 5
    });
  });

  it('inserts paired inline markers at a cursor', () => {
    expect(applySourceFormatting('hello', { anchor: 5, head: 5 }, 'inlineCode')).toEqual({
      text: 'hello``',
      anchor: 6,
      head: 6
    });
  });

  it('toggles and renumbers prefixes across selected lines', () => {
    const listed = applySourceFormatting('alpha\nbeta', { anchor: 0, head: 10 }, 'orderedList');
    expect(listed.text).toBe('1. alpha\n2. beta');
    expect(applySourceFormatting(listed.text, listed, 'orderedList').text).toBe('alpha\nbeta');
  });

  it('wraps and unwraps fenced code blocks', () => {
    const fenced = applySourceFormatting('alpha\nbeta', { anchor: 0, head: 10 }, 'codeBlock');
    expect(fenced.text).toBe('```\nalpha\nbeta\n```');
    expect(applySourceFormatting(fenced.text, { anchor: 6, head: 6 }, 'codeBlock').text).toBe(
      'alpha\nbeta'
    );
  });
});

describe('getSourceFormattingState', () => {
  it('derives active marks and structures from the Markdown syntax tree', () => {
    const state = EditorState.create({
      doc: '> ## **bold** and `code`',
      selection: { anchor: 9 },
      extensions: [markdown()]
    });
    expect(getSourceFormattingState(state)).toMatchObject({
      bold: true,
      heading: true,
      blockquote: true
    });

    const codeState = state.update({ selection: { anchor: 21 } }).state;
    expect(getSourceFormattingState(codeState).inlineCode).toBe(true);
  });
});
