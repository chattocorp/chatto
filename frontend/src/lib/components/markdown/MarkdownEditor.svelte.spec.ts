import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import MarkdownEditor, { type MarkdownEditorApi } from './MarkdownEditor.svelte';

async function findEditor(container: Element): Promise<HTMLElement> {
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="markdown-editor"]')).toBeTruthy()
  );
  return container.querySelector('[data-testid="markdown-editor"]') as HTMLElement;
}

async function insertText(editor: HTMLElement, text: string) {
  editor.focus();
  document.execCommand('insertText', false, text);
  await tick();
}

async function pressKey(editor: HTMLElement, key: string) {
  editor.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  await tick();
}

describe('MarkdownEditor', () => {
  it('serializes separate paragraphs with blank markdown lines', async () => {
    const updates: string[] = [];
    const { container } = render(MarkdownEditor, {
      props: {
        testid: 'markdown-editor',
        onUpdate: (markdown) => updates.push(markdown)
      }
    });
    const editor = await findEditor(container);

    await insertText(editor, 'First paragraph');
    await pressKey(editor, 'Enter');
    await insertText(editor, 'Second paragraph');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('First paragraph\n\nSecond paragraph'));
  });

  it('preserves blank lines when editing restored markdown', async () => {
    const updates: string[] = [];
    let api: MarkdownEditorApi | null = null;
    const { container } = render(MarkdownEditor, {
      props: {
        testid: 'markdown-editor',
        onUpdate: (markdown) => updates.push(markdown),
        onReady: (editorApi) => {
          api = editorApi;
          editorApi.setContent('First paragraph\n\nSecond paragraph');
        }
      }
    });
    const editor = await findEditor(container);

    await vi.waitFor(() => expect(api).toBeTruthy());
    await vi.waitFor(() => expect(editor.querySelectorAll('p')).toHaveLength(2));
    editor.focus();
    await insertText(editor, '!');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('!First paragraph\n\nSecond paragraph'));
  });

  it('preserves visual empty paragraphs when editing restored markdown', async () => {
    const updates: string[] = [];
    let api: MarkdownEditorApi | null = null;
    const { container } = render(MarkdownEditor, {
      props: {
        testid: 'markdown-editor',
        onUpdate: (markdown) => updates.push(markdown),
        onReady: (editorApi) => {
          api = editorApi;
          editorApi.setContent('Stuff\n\n\n\nNo Stuff');
        }
      }
    });
    const editor = await findEditor(container);

    await vi.waitFor(() => expect(api).toBeTruthy());
    await vi.waitFor(() => expect(editor.querySelectorAll('p')).toHaveLength(3));
    editor.focus();
    await insertText(editor, '!');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('!Stuff\n\n\n\nNo Stuff'));
  });

  it('serializes a blank line before a list after hard breaks', async () => {
    const updates: string[] = [];
    const { container } = render(MarkdownEditor, {
      props: {
        testid: 'markdown-editor',
        onUpdate: (markdown) => updates.push(markdown)
      }
    });
    const editor = await findEditor(container);

    await insertText(editor, 'Things I hate:');
    await pressKey(editor, 'Enter');
    await pressKey(editor, 'Enter');
    await insertText(editor, '- lists');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('Things I hate:\n\n\n\n- lists'));
  });
});
