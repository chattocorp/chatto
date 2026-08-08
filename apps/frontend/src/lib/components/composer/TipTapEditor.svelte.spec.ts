import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import '../../../app.css';
import TipTapEditor from './TipTapEditor.svelte';

describe('TipTapEditor accessibility', () => {
  it('keeps its accessible name synchronized with the placeholder', async () => {
    const rendered = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    await rendered.rerender({ placeholder: 'Edit your message' });

    await expect.element(page.getByRole('textbox', { name: 'Edit your message' })).toBeVisible();
  });
});

describe('TipTapEditor wrapping', () => {
  it('uses stable wrapping instead of global prose wrapping', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const paragraph = container.querySelector('.ProseMirror p');
    expect(paragraph).toBeInstanceOf(HTMLParagraphElement);
    expect(getComputedStyle(paragraph!).textWrap).toBe('wrap');
  });

  it('uses logical prose edges and isolates code as LTR', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const editor = container.querySelector('.ProseMirror');
    expect(editor).toBeInstanceOf(HTMLElement);
    if (!editor) return;

    const quote = document.createElement('blockquote');
    quote.textContent = 'مرحبا';
    const code = document.createElement('pre');
    code.textContent = 'const direction = "ltr";';
    const inlineCode = document.createElement('code');
    inlineCode.textContent = 'const direction = "ltr";';
    editor.append(quote, code, inlineCode);

    const quoteStyle = getComputedStyle(quote);
    expect(quoteStyle.borderInlineStartWidth).toBe('3px');
    expect(quoteStyle.paddingInlineStart).toBe('14.4px');
    expect(quoteStyle.unicodeBidi).toBe('plaintext');
    expect(getComputedStyle(code).direction).toBe('ltr');
    expect(getComputedStyle(code).unicodeBidi).toBe('isolate');
    expect(getComputedStyle(inlineCode).direction).toBe('ltr');
    expect(getComputedStyle(inlineCode).unicodeBidi).toBe('isolate');
  });

  it('aligns RTL ordered-list markers toward their content without start padding', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const editor = container.querySelector('.ProseMirror');
    expect(editor).toBeInstanceOf(HTMLElement);
    if (!editor) return;

    const list = document.createElement('ol');
    const item = document.createElement('li');
    item.textContent = 'العنصر الأول';
    list.append(item);
    editor.setAttribute('dir', 'rtl');
    editor.append(list);

    expect(getComputedStyle(list).paddingInlineStart).toBe('0px');
    expect(getComputedStyle(item, '::before').textAlign).toBe('end');
  });
});
