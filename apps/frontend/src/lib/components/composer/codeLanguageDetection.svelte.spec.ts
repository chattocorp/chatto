import { afterEach, describe, expect, it, vi } from 'vitest';
import { Schema } from '@tiptap/pm/model';
import { EditorState, TextSelection } from '@tiptap/pm/state';
import { EditorView } from '@tiptap/pm/view';
import { Editor } from '@tiptap/core';
import { createComposerExtensions } from './extensions';
import { getSerializedMarkdown } from './markdown';
import { renderMarkdown } from '$lib/markdown';
import { ensureCodeLanguageLoaded } from '$lib/codeHighlighting';
import { createCodeLanguageDetectionPlugin } from './codeLanguageDetection';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    codeBlock: {
      content: 'text*',
      group: 'block',
      code: true,
      attrs: { language: { default: null } },
      toDOM: () => ['pre', ['code', 0]]
    },
    paragraph: { content: 'text*', group: 'block', toDOM: () => ['p', 0] },
    text: {}
  }
});
const views: EditorView[] = [];

function setup(language: string | null = null, text = '') {
  let resolve!: (language: string | null) => void;
  const detect = vi.fn(
    () =>
      new Promise<string | null>((done) => {
        resolve = done;
      })
  );
  const mount = document.body.appendChild(document.createElement('div'));
  const doc = schema.node('doc', null, [
    schema.node('codeBlock', { language }, text ? schema.text(text) : undefined),
    schema.node('paragraph')
  ]);
  const view = new EditorView(mount, {
    state: EditorState.create({
      schema,
      doc,
      selection: TextSelection.create(doc, 1),
      plugins: [createCodeLanguageDetectionPlugin(detect)]
    })
  });
  views.push(view);
  return { view, detect, resolve: (language: string | null) => resolve(language) };
}

afterEach(() => {
  for (const view of views.splice(0)) {
    const mount = view.dom.parentElement;
    view.destroy();
    mount?.remove();
  }
});

describe('composer code language persistence', () => {
  it('round trips a detected language and matches posted highlight tokens', async () => {
    const editor = new Editor({
      extensions: createComposerExtensions('Code'),
      content: '<pre><code></code></pre>'
    });
    try {
      editor.commands.setTextSelection(1);
      editor.view.dispatch(
        editor.state.tr
          .insertText(
            '19:38:55 [worker-3438 / task-3] Choose Chatto participation\n19:38:56 Decision: ignore · probabilities: {"engage":0.19}'
          )
          .setMeta('uiEvent', 'paste')
      );
      await vi.waitFor(() => expect(editor.state.doc.firstChild?.attrs.language).toBeTruthy());
      const markdown = getSerializedMarkdown(editor);
      const language = editor.state.doc.firstChild!.attrs.language;
      expect(markdown).toContain('```' + language + '\n');
      const rendered = document.createElement('div');
      rendered.innerHTML = await renderMarkdown(markdown);
      const tokens = (element: Element) =>
        [...element.querySelectorAll('code [class^="hljs-"]')].map((span) => [
          span.className,
          span.textContent
        ]);
      expect(tokens(editor.view.dom).length).toBeGreaterThan(0);
      expect(tokens(rendered)).toEqual(tokens(editor.view.dom));
      editor.commands.setContent(markdown, { contentType: 'markdown', emitUpdate: false });
      expect(editor.state.doc.firstChild?.attrs.language).toBe(language);
      expect(getSerializedMarkdown(editor)).toBe(markdown);
    } finally {
      editor.destroy();
    }
  });

  it.each(['text', 'notalanguage', null])('does not decorate %s implicitly', async (language) => {
    await ensureCodeLanguageLoaded('javascript');
    const editor = new Editor({
      extensions: createComposerExtensions('Code'),
      content: {
        type: 'doc',
        content: [
          {
            type: 'codeBlock',
            attrs: { language },
            content: [{ type: 'text', text: 'const answer = 42;' }]
          }
        ]
      }
    });
    try {
      expect(editor.view.dom.querySelector('[class^="hljs-"]')).toBeNull();
    } finally {
      editor.destroy();
    }
  });
});

describe('code language detection', () => {
  it.each(['moved', 'decoration refresh'])(
    'keeps a pending guess when unchanged code is %s',
    async (change) => {
      const { view, detect, resolve } = setup();
      view.dispatch(view.state.tr.insertText('const answer = 42;').setMeta('uiEvent', 'paste'));
      if (change === 'moved') {
        view.dispatch(view.state.tr.insert(0, schema.node('paragraph')));
      } else {
        view.dispatch(view.state.tr.setNodeMarkup(0, undefined, view.state.doc.firstChild!.attrs));
      }
      resolve('javascript');
      await vi.waitFor(() =>
        expect(view.state.doc.nodeAt(change === 'moved' ? 2 : 0)?.attrs.language).toBe('javascript')
      );
      expect(detect).toHaveBeenCalledOnce();
    }
  );

  it('saves a guess after paste without moving the selection', async () => {
    const { view, detect, resolve } = setup();
    view.dispatch(view.state.tr.insertText('const answer = 42;').setMeta('uiEvent', 'paste'));
    expect(detect).toHaveBeenCalledExactlyOnceWith('const answer = 42;');
    const selection = view.state.selection.from;
    resolve('javascript');
    await vi.waitFor(() => expect(view.state.doc.firstChild?.attrs.language).toBe('javascript'));
    expect(view.state.selection.from).toBe(selection);
  });

  it.each(['selection', 'blur'])('detects typed code on block exit through %s', async (exit) => {
    const { view, detect, resolve } = setup();
    view.dispatch(view.state.tr.insertText('const answer = 42;'));
    expect(detect).not.toHaveBeenCalled();
    if (exit === 'blur') view.dom.dispatchEvent(new FocusEvent('blur'));
    else
      view.dispatch(
        view.state.tr.setSelection(
          TextSelection.create(view.state.doc, view.state.doc.firstChild!.nodeSize + 1)
        )
      );
    expect(detect).toHaveBeenCalledOnce();
    resolve('javascript');
    await vi.waitFor(() => expect(view.state.doc.firstChild?.attrs.language).toBe('javascript'));
  });

  it.each(['text', 'go', 'notalanguage'])('preserves explicit %s', (language) => {
    const { view, detect } = setup(language);
    view.dispatch(view.state.tr.insertText('const answer = 42;').setMeta('uiEvent', 'paste'));
    view.dom.dispatchEvent(new FocusEvent('blur'));
    expect(detect).not.toHaveBeenCalled();
  });

  it('does not detect existing code on focus, selection, or draft load', () => {
    const { view, detect } = setup(null, 'const answer = 42;');
    view.dom.dispatchEvent(new FocusEvent('blur'));
    view.dispatch(view.state.tr.insertText('changed').setMeta('preventUpdate', true));
    view.dom.dispatchEvent(new FocusEvent('blur'));
    expect(detect).not.toHaveBeenCalled();
  });

  it.each(['content', 'language', 'removed', 'draft', 'disposed'])(
    'discards a result after %s changes',
    async (change) => {
      const { view, resolve } = setup();
      view.dispatch(view.state.tr.insertText('const answer = 42;').setMeta('uiEvent', 'paste'));
      if (change === 'content') view.dispatch(view.state.tr.insertText('more'));
      if (change === 'language')
        view.dispatch(view.state.tr.setNodeMarkup(0, undefined, { language: 'text' }));
      if (change === 'removed')
        view.dispatch(view.state.tr.delete(0, view.state.doc.firstChild!.nodeSize));
      if (change === 'draft') view.dispatch(view.state.tr.setMeta('preventUpdate', true));
      if (change === 'disposed') view.destroy();
      const before = view.state.doc;
      resolve('javascript');
      await Promise.resolve();
      expect(view.state.doc).toBe(before);
    }
  );

  it('keeps a failed guess plain and does not retry it on each selection change', async () => {
    const { view, detect, resolve } = setup();
    view.dispatch(view.state.tr.insertText('hello').setMeta('uiEvent', 'paste'));
    resolve(null);
    await Promise.resolve();
    view.dom.dispatchEvent(new FocusEvent('blur'));
    expect(view.state.doc.firstChild?.attrs.language).toBeNull();
    expect(detect).toHaveBeenCalledOnce();
  });

  it('does not detect empty or whitespace-only code', () => {
    const { view, detect } = setup();
    view.dispatch(view.state.tr.insertText('  ').setMeta('uiEvent', 'paste'));
    view.dom.dispatchEvent(new FocusEvent('blur'));
    expect(detect).not.toHaveBeenCalled();
  });
});
