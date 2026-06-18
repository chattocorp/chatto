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
    body.append('quoted text');
    document.body.append(body);
    const textNode = body.firstChild!;

    const selection = selectText(textNode, 0, textNode, 'quoted'.length);

    expect(selectedQuoteTextForMessageBody(selection, body)).toBe('quoted');
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
