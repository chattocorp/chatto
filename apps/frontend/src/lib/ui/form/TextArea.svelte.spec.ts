import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import TextArea from './TextArea.svelte';

function insert(textarea: HTMLTextAreaElement, value: string, inputType = 'insertText') {
  const event = new InputEvent('beforeinput', {
    bubbles: true,
    cancelable: true,
    data: inputType === 'insertLineBreak' ? null : value,
    inputType
  });
  textarea.dispatchEvent(event);
  if (event.defaultPrevented) return;

  textarea.setRangeText(value, textarea.selectionStart, textarea.selectionEnd, 'end');
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('TextArea', () => {
  it('enforces a UTF-8 byte limit', () => {
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      value: 'draft',
      maxBytes: 5
    });
    const textarea = container.querySelector('textarea')!;

    expect(textarea.maxLength).toBe(5);

    textarea.select();
    insert(textarea, 'draft!');
    expect(textarea.value).toBe('draft');
  });

  it('accepts multibyte input at the limit and rejects the next edit', () => {
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      maxBytes: 4
    });
    const textarea = container.querySelector('textarea')!;

    insert(textarea, '💬');
    expect(textarea.value).toBe('💬');

    insert(textarea, 'a');
    expect(textarea.value).toBe('💬');
  });

  it('keeps the part of a middle insertion that fits, and the existing suffix', () => {
    const existingDraft = `${'💬'.repeat(123)}suffix`;
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      value: existingDraft,
      maxBytes: 500
    });
    const textarea = container.querySelector('textarea')!;
    textarea.setSelectionRange(10, 10);

    insert(textarea, 'inserted');

    expect(textarea.value).toBe(`${existingDraft.slice(0, 10)}in${existingDraft.slice(10)}`);
    expect(textarea.selectionStart).toBe(12);
  });

  it('keeps the part of a pasted text that fits, in whole characters', () => {
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      value: 'ab',
      maxBytes: 12
    });
    const textarea = container.querySelector('textarea')!;
    textarea.setSelectionRange(2, 2);

    // The family emoji is one character of 25 bytes and cannot be split.
    insert(textarea, 'cd👨‍👩‍👧‍👦ef', 'insertFromPaste');

    expect(textarea.value).toBe('abcd');
  });

  it('counts inserted line breaks toward the byte limit', () => {
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      value: '1234',
      maxBytes: 4
    });
    const textarea = container.querySelector('textarea')!;

    insert(textarea, '\n', 'insertLineBreak');

    expect(textarea.value).toBe('1234');
  });

  it('restores the last accepted value when beforeinput cannot prevent an edit', () => {
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      value: '💬',
      maxBytes: 4
    });
    const textarea = container.querySelector('textarea')!;

    textarea.value = '💬a';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));

    expect(textarea.value).toBe('💬');
  });
});
