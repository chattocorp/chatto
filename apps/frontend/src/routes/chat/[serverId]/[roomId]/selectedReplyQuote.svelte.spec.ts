import { afterEach, describe, expect, it } from 'vitest';
import { normalizeSelectedQuoteText, selectedQuoteTextForMessageBody } from './selectedReplyQuote';

function selectText(startNode: Node, startOffset: number, endNode: Node, endOffset: number) {
  const range = document.createRange();
  range.setStart(startNode, startOffset);
  range.setEnd(endNode, endOffset);

  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
  return selection;
}

describe('selected reply quotes', () => {
  afterEach(() => {
    window.getSelection()?.removeAllRanges();
    document.body.replaceChildren();
  });

  it('normalizes selected quote text', () => {
    expect(normalizeSelectedQuoteText(' \r\n first\rsecond\n ')).toBe('first\nsecond');
    expect(normalizeSelectedQuoteText('   \n\t  ')).toBeNull();
  });

  it('returns selected text when both endpoints are inside the message body', () => {
    const body = document.createElement('div');
    body.innerHTML = '<p>quoted text</p>';
    document.body.append(body);
    const textNode = body.querySelector('p')!.firstChild!;

    const selection = selectText(textNode, 0, textNode, 'quoted'.length);

    expect(selectedQuoteTextForMessageBody(selection, body)).toEqual([
      { quoteDepth: 0, text: 'quoted' }
    ]);
  });

  it('returns quote depth for selected text inside a blockquote', () => {
    const body = document.createElement('div');
    body.innerHTML = '<blockquote><p>quoted text</p></blockquote>';
    document.body.append(body);
    const textNode = body.querySelector('p')!.firstChild!;

    const selection = selectText(textNode, 0, textNode, 'quoted'.length);

    expect(selectedQuoteTextForMessageBody(selection, body)).toEqual([
      { quoteDepth: 1, text: 'quoted' }
    ]);
  });

  it('preserves mixed normal and quoted selected text as separate quote blocks', () => {
    const body = document.createElement('div');
    body.innerHTML = '<p>a</p><blockquote><p>nice, love it</p></blockquote><p>:D</p>';
    document.body.append(body);
    const firstTextNode = body.querySelector('p')!.firstChild!;
    const lastTextNode = body.children[2].firstChild!;

    const selection = selectText(firstTextNode, 0, lastTextNode, ':D'.length);

    expect(selectedQuoteTextForMessageBody(selection, body)).toEqual([
      { quoteDepth: 0, text: 'a' },
      { quoteDepth: 1, text: 'nice, love it' },
      { quoteDepth: 0, text: ':D' }
    ]);
  });

  it('ignores selections that leave the message body', () => {
    const body = document.createElement('div');
    body.append('message text');
    const outside = document.createElement('div');
    outside.append('outside text');
    document.body.append(body, outside);

    const selection = selectText(body.firstChild!, 0, outside.firstChild!, 'outside'.length);

    expect(selectedQuoteTextForMessageBody(selection, body)).toBeNull();
  });
});
