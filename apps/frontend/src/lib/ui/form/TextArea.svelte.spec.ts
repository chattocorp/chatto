import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import TextArea from './TextArea.svelte';

describe('TextArea', () => {
  it('cuts text at the end to fit a UTF-8 byte limit', () => {
    const { container } = render(TextArea, {
      id: 'description',
      label: 'Description',
      value: 'ab',
      maxBytes: 6
    });
    const textarea = container.querySelector('textarea')!;
    expect(textarea.maxLength).toBe(6);

    textarea.value = 'ab💬cd';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));

    expect(textarea.value).toBe('ab💬');
  });
});
