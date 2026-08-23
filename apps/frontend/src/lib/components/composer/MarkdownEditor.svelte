<!--
@component

CodeMirror-backed Markdown source editor for the message composer. It exposes
the same API as the visual editor while keeping the stored Markdown visible.
-->
<script lang="ts">
  import { tick, untrack } from 'svelte';
  import {
    defaultKeymap,
    history,
    historyKeymap,
    indentLess,
    indentMore,
    indentWithTab,
    insertNewlineAndIndent,
    simplifySelection,
    temporarilySetTabFocusMode
  } from '@codemirror/commands';
  import { syntaxHighlighting, HighlightStyle } from '@codemirror/language';
  import {
    commonmarkLanguage,
    insertNewlineContinueMarkup,
    markdown
  } from '@codemirror/lang-markdown';
  import { Compartment, EditorSelection, EditorState, Prec, Transaction } from '@codemirror/state';
  import {
    drawSelection,
    EditorView,
    keymap,
    placeholder as editorPlaceholder
  } from '@codemirror/view';
  import { tags } from '@lezer/highlight';
  import { Autolink, Table } from '@lezer/markdown';
  import { m } from '$lib/i18n/messages';
  import { codeFenceHighlighting } from './codeFenceHighlighting';
  import type {
    ComposerEditorApi,
    ComposerEditorProps,
    ComposerFormattingState,
    ComposerIndentState
  } from './editorTypes';
  import { emptyComposerIndentState } from './editorTypes';
  import { serializeQuoteInsertionContent } from './quotes';
  import { applySourceFormatting, getSourceFormattingState } from './sourceFormatting';

  const emptyFormattingState: ComposerFormattingState = {
    bold: false,
    italic: false,
    inlineCode: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    blockquote: false,
    codeBlock: false
  };

  const sourceIndentState: ComposerIndentState = {
    canIndent: true,
    canOutdent: true
  };

  const escapeWithTabFocus = (editorView: EditorView): boolean => {
    simplifySelection(editorView);
    return temporarilySetTabFocusMode(editorView);
  };

  const markdownHighlightStyle = HighlightStyle.define([
    { tag: tags.heading, color: 'var(--color-text)', fontWeight: '600' },
    { tag: tags.strong, fontWeight: '700' },
    { tag: tags.emphasis, fontStyle: 'italic' },
    { tag: [tags.link, tags.url], color: 'var(--color-link)', textDecoration: 'underline' },
    { tag: [tags.list, tags.quote], color: 'var(--color-muted)' },
    {
      tag: tags.monospace,
      color: 'var(--color-text)',
      backgroundColor: 'var(--color-surface-emphasized)',
      fontFamily: 'var(--font-mono)'
    },
    { tag: [tags.processingInstruction, tags.punctuation], color: 'var(--color-muted)' }
  ]);

  const markdownTheme = EditorView.theme({
    '&': {
      minHeight: '2rem',
      maxHeight: '12.5rem',
      backgroundColor: 'transparent',
      color: 'var(--color-text)',
      fontSize: '16px'
    },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': {
      overflowX: 'hidden',
      overflowY: 'auto',
      fontFamily: 'var(--font-sans)',
      lineHeight: '1.5'
    },
    '.cm-content': {
      minHeight: '2rem',
      padding: '0.25rem 0',
      caretColor: 'var(--color-text)'
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: 'var(--color-text)'
    },
    '.cm-line': { padding: '0' },
    '.cm-placeholder': { color: 'var(--color-muted)', fontStyle: 'normal' },
    '.cm-code-fence': {
      boxSizing: 'border-box',
      backgroundColor: 'color-mix(in srgb, var(--color-surface-emphasized) 68%, transparent)',
      paddingInline: '0.5rem'
    },
    '.cm-code-fence-open': {
      marginTop: '0.25rem',
      borderStartStartRadius: '0.375rem',
      borderStartEndRadius: '0.375rem',
      paddingTop: '0.2rem',
      color: 'var(--color-muted)'
    },
    '.cm-code-fence-body': {
      fontFamily: 'var(--font-mono)',
      direction: 'ltr',
      unicodeBidi: 'isolate',
      color: 'var(--composer-code-text)'
    },
    '.cm-code-fence-body span': {
      backgroundColor: 'transparent'
    },
    '.cm-code-fence-body span:not([class*="hljs-"])': {
      color: 'inherit'
    },
    '.cm-code-fence-close': {
      marginBottom: '0.25rem',
      borderEndStartRadius: '0.375rem',
      borderEndEndRadius: '0.375rem',
      paddingBottom: '0.2rem',
      color: 'var(--color-muted)'
    },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, ::selection':
      {
        backgroundColor: 'color-mix(in srgb, var(--color-action) 20%, transparent)'
      },
    '.hljs-comment, .hljs-quote': {
      color: 'var(--composer-code-comment)',
      fontStyle: 'italic'
    },
    '.hljs-keyword, .hljs-selector-tag, .hljs-subst': {
      color: 'var(--composer-code-keyword)'
    },
    '.hljs-string, .hljs-regexp, .hljs-symbol, .hljs-bullet': {
      color: 'var(--composer-code-string)'
    },
    '.hljs-title, .hljs-section, .hljs-name, .hljs-selector-id, .hljs-selector-class': {
      color: 'var(--composer-code-title)'
    },
    '.hljs-number, .hljs-literal, .hljs-type, .hljs-built_in': {
      color: 'var(--composer-code-literal)'
    },
    '.hljs-attr, .hljs-attribute, .hljs-variable, .hljs-template-variable': {
      color: 'var(--composer-code-attribute)'
    }
  });

  let {
    placeholder = m('composer.placeholder'),
    editable = true,
    autofocus = false,
    testid,
    onUpdate,
    onKeyDown,
    onPaste,
    onFormattingStateChange,
    onIndentStateChange,
    onReady,
    onDestroy
  }: ComposerEditorProps = $props();

  let view: EditorView | null = null;

  function mountEditor(node: HTMLDivElement) {
    const editableCompartment = new Compartment();
    const placeholderCompartment = new Compartment();
    const attributesCompartment = new Compartment();
    const initial = untrack(() => ({ editable, placeholder, testid, autofocus }));
    let destroyed = false;
    let suppressUpdate = false;

    const editorView = new EditorView({
      parent: node,
      state: EditorState.create({
        extensions: [
          history(),
          drawSelection(),
          keymap.of([
            { key: 'Escape', run: escapeWithTabFocus },
            indentWithTab,
            ...historyKeymap,
            ...defaultKeymap
          ]),
          markdown({
            base: commonmarkLanguage,
            extensions: [Table, Autolink],
            completeHTMLTags: false,
            pasteURLAsLink: false
          }),
          syntaxHighlighting(markdownHighlightStyle),
          codeFenceHighlighting,
          markdownTheme,
          EditorView.lineWrapping,
          EditorView.perLineTextDirection.of(true),
          editableCompartment.of([
            EditorView.editable.of(initial.editable),
            EditorState.readOnly.of(!initial.editable)
          ]),
          placeholderCompartment.of(editorPlaceholder(initial.placeholder)),
          attributesCompartment.of(contentAttributes(initial.placeholder, initial.testid)),
          Prec.highest(
            EditorView.domEventHandlers({
              keydown: (event) => {
                if (!onKeyDown?.(event)) return false;
                event.preventDefault();
                return true;
              },
              paste: (event) => {
                if (!onPaste?.(event)) return false;
                event.preventDefault();
                return true;
              }
            })
          ),
          EditorView.updateListener.of((update) => {
            if (update.docChanged && !suppressUpdate) onUpdate?.(update.state.doc.toString());
            if (update.docChanged || update.selectionSet) {
              onFormattingStateChange?.(getSourceFormattingState(update.state));
            }
          })
        ]
      })
    });
    view = editorView;

    const api: ComposerEditorApi = {
      getText: () => (destroyed ? '' : editorView.state.doc.toString()),
      setContent: (source) => {
        if (destroyed || source === editorView.state.doc.toString()) return;
        suppressUpdate = true;
        editorView.dispatch({
          changes: { from: 0, to: editorView.state.doc.length, insert: source },
          selection: EditorSelection.cursor(source.length),
          annotations: Transaction.addToHistory.of(false)
        });
        suppressUpdate = false;
        publishFormattingState(editorView);
      },
      focus: (position = 'end') => {
        if (destroyed) return;
        const cursor = position === 'start' ? 0 : editorView.state.doc.length;
        editorView.dispatch({ selection: EditorSelection.cursor(cursor), scrollIntoView: true });
        editorView.focus();
      },
      performEnter: () => {
        if (destroyed) return;
        if (!insertNewlineContinueMarkup(editorView)) insertNewlineAndIndent(editorView);
        editorView.focus();
      },
      getTextBeforeCursor: () =>
        destroyed ? '' : editorView.state.doc.sliceString(0, editorView.state.selection.main.head),
      isInCodeBlock: () => !destroyed && getSourceFormattingState(editorView.state).codeBlock,
      replaceTextBeforeCursor: (charCount, replacement) => {
        if (destroyed) return;
        const head = editorView.state.selection.main.head;
        const from = Math.max(0, head - charCount);
        editorView.dispatch({
          changes: { from, to: head, insert: replacement },
          selection: EditorSelection.cursor(from + replacement.length),
          scrollIntoView: true
        });
        editorView.focus();
      },
      insertText: (text) => {
        if (destroyed || !text) return;
        editorView.dispatch(editorView.state.replaceSelection(text));
        editorView.focus();
      },
      toggleFormatting: (command) => {
        if (destroyed) return;
        const main = editorView.state.selection.main;
        const result = applySourceFormatting(
          editorView.state.doc.toString(),
          { anchor: main.anchor, head: main.head },
          command
        );
        editorView.dispatch({
          changes: { from: 0, to: editorView.state.doc.length, insert: result.text },
          selection: EditorSelection.single(result.anchor, result.head),
          scrollIntoView: true
        });
        editorView.focus();
      },
      adjustIndent: (direction) => {
        if (destroyed) return false;
        const applied = direction === 'indent' ? indentMore(editorView) : indentLess(editorView);
        if (!applied) return false;
        editorView.focus();
        return true;
      },
      insertQuote: (content) => {
        if (destroyed) return;
        const quote = serializeQuoteInsertionContent(content);
        if (!quote) return;
        const selection = editorView.state.selection.main;
        const before = editorView.state.doc.sliceString(0, selection.from);
        const after = editorView.state.doc.sliceString(selection.to);
        const prefix =
          before.length === 0
            ? ''
            : before.endsWith('\n\n')
              ? ''
              : before.endsWith('\n')
                ? '\n'
                : '\n\n';
        const suffix =
          after.length === 0
            ? '\n\n'
            : after.startsWith('\n\n')
              ? ''
              : after.startsWith('\n')
                ? '\n'
                : '\n\n';
        const inserted = `${prefix}${quote}${suffix}`;
        editorView.dispatch({
          changes: { from: selection.from, to: selection.to, insert: inserted },
          selection: EditorSelection.cursor(selection.from + inserted.length),
          scrollIntoView: true
        });
        editorView.focus();
      }
    };

    $effect(() => {
      editorView.dispatch({
        effects: editableCompartment.reconfigure([
          EditorView.editable.of(editable),
          EditorState.readOnly.of(!editable)
        ])
      });
    });

    $effect(() => {
      editorView.dispatch({
        effects: placeholderCompartment.reconfigure(editorPlaceholder(placeholder))
      });
    });

    $effect(() => {
      editorView.dispatch({
        effects: attributesCompartment.reconfigure(contentAttributes(placeholder, testid))
      });
    });

    publishFormattingState(editorView);
    tick().then(() => {
      if (destroyed) return;
      onReady?.(api);
      if (initial.autofocus) api.focus();
    });

    return () => {
      destroyed = true;
      onFormattingStateChange?.(emptyFormattingState);
      onIndentStateChange?.(emptyComposerIndentState);
      onDestroy?.(api);
      editorView.destroy();
      if (view === editorView) view = null;
    };
  }

  function contentAttributes(label: string, dataTestid?: string) {
    return EditorView.contentAttributes.of({
      role: 'textbox',
      'aria-label': label,
      'aria-multiline': 'true',
      spellcheck: 'true',
      autocapitalize: 'sentences',
      ...(dataTestid ? { 'data-testid': dataTestid } : {})
    });
  }

  function publishFormattingState(editorView: EditorView): void {
    onFormattingStateChange?.(getSourceFormattingState(editorView.state));
    onIndentStateChange?.(sourceIndentState);
  }
</script>

<div
  {@attach mountEditor}
  data-composer-editor="markdown"
  class="composer-code-palette min-h-8 min-w-0 flex-1"
  class:cursor-not-allowed={!editable}
></div>
