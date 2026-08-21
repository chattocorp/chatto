import { EditorState } from '@codemirror/state';
import { markdown } from '@codemirror/lang-markdown';
import { describe, expect, it } from 'vitest';
import {
  adjustSourceListIndent,
  applySourceFormatting,
  getSourceFormattingState,
  getSourceListIndentState
} from './sourceFormatting';

function sourceState(doc: string, anchor: number, head = anchor) {
  return EditorState.create({
    doc,
    selection: { anchor, head },
    extensions: [markdown()]
  });
}

describe('applySourceFormatting', () => {
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

describe('Markdown list indentation', () => {
  it('indents under a previous sibling and outdents by one nesting level', () => {
    const source = '- first\n- second';
    const state = sourceState(source, source.indexOf('second'));

    expect(getSourceListIndentState(state)).toEqual({ canIndent: true, canOutdent: true });
    const indented = adjustSourceListIndent(state, 'indent');
    expect(indented).toEqual({
      text: '- first\n  - second',
      anchor: source.indexOf('second') + 2,
      head: source.indexOf('second') + 2
    });

    const nestedState = sourceState(indented!.text, indented!.anchor);
    expect(getSourceListIndentState(nestedState)).toEqual({
      canIndent: false,
      canOutdent: true
    });
    expect(adjustSourceListIndent(nestedState, 'outdent')).toEqual({
      text: source,
      anchor: source.indexOf('second'),
      head: source.indexOf('second')
    });
  });

  it('lifts top-level items out of their list like the visual editor', () => {
    const source = '- first\n- second';
    const second = sourceState(source, source.indexOf('second'));
    const liftedSecond = adjustSourceListIndent(second, 'outdent');
    expect(liftedSecond).toEqual({
      text: '- first\n\nsecond',
      anchor: 9,
      head: 9
    });

    const first = sourceState(source, source.indexOf('first'));
    expect(adjustSourceListIndent(first, 'outdent')?.text).toBe('first\n\n- second');
  });

  it('keeps a lifted list item inside its enclosing blockquote', () => {
    const source = '> - first\n> - second';
    const state = sourceState(source, source.indexOf('second'));

    expect(adjustSourceListIndent(state, 'outdent')?.text).toBe('> - first\n>\n> second');
  });

  it('turns multiple lifted root items into separate paragraphs', () => {
    const source = '- first\n- second\n- third';
    const state = sourceState(source, source.indexOf('second'), source.length);

    expect(adjustSourceListIndent(state, 'outdent')?.text).toBe('- first\n\nsecond\n\nthird');
  });

  it('uses the parent marker width for multi-digit ordered lists', () => {
    const source = '100. parent\n101. child';
    const state = sourceState(source, source.indexOf('child'));

    expect(adjustSourceListIndent(state, 'indent')?.text).toBe('100. parent\n     101. child');
  });

  it('moves selected sibling items together with their descendants', () => {
    const source = '- first\n- second\n  - child\n- third';
    const state = sourceState(source, source.indexOf('- second'), source.length);
    const result = adjustSourceListIndent(state, 'indent');

    expect(result?.text).toBe('- first\n  - second\n    - child\n  - third');
    expect(result?.anchor).toBe(source.indexOf('- second') + 2);
    expect(result?.head).toBe(source.length + 6);
  });

  it('places indentation after an enclosing blockquote marker', () => {
    const source = '> - first\n> - second';
    const state = sourceState(source, source.indexOf('second'));
    const indented = adjustSourceListIndent(state, 'indent');

    expect(indented?.text).toBe('> - first\n>   - second');
    const nestedState = sourceState(indented!.text, indented!.anchor);
    expect(adjustSourceListIndent(nestedState, 'outdent')?.text).toBe(source);
  });

  it('does not capture indentation outside a structurally applicable list item', () => {
    const firstItem = sourceState('- first\n- second', 2);
    const prose = sourceState('plain text', 5);

    expect(getSourceListIndentState(firstItem)).toEqual({
      canIndent: false,
      canOutdent: true
    });
    expect(adjustSourceListIndent(firstItem, 'indent')).toBeNull();
    expect(adjustSourceListIndent(prose, 'indent')).toBeNull();
    expect(adjustSourceListIndent(prose, 'outdent')).toBeNull();
  });
});
