<!--
@component

Lightweight TipTap editor wrapper for chat input. Manages editor lifecycle
and exposes a typed API for text manipulation (mentions, emoji, drafts).

**Props:**
- `placeholder` - Placeholder text shown when editor is empty
- `editable` - Whether the editor accepts input
- `autofocus` - Focus editor on mount
- `testid` - data-testid attribute for E2E testing
- `onUpdate` - Called with plain text content on each change
- `onKeyDown` - Keyboard event handler; return true to prevent TipTap default
- `onPaste` - Paste event handler; return true to prevent TipTap default
- `onReady` - Called with editor API when editor is initialized
-->
<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { Editor } from '@tiptap/core';
  import StarterKit from '@tiptap/starter-kit';
  import Placeholder from '@tiptap/extension-placeholder';

  export type TipTapEditorApi = {
    /** Get the editor's plain text content */
    getText: () => string;
    /** Set editor content from plain text */
    setContent: (text: string) => void;
    /** Focus the editor */
    focus: (position?: 'start' | 'end') => void;
    /** Get plain text from document start to cursor position */
    getTextBeforeCursor: () => string;
    /**
     * Replace N characters before the cursor with new text.
     * Used for mention/emoji completion where we know the pattern
     * length relative to the cursor.
     */
    replaceTextBeforeCursor: (charCount: number, replacement: string) => void;
  };

  let {
    placeholder = 'Type a message...',
    editable = true,
    autofocus = false,
    testid,
    onUpdate,
    onKeyDown,
    onPaste,
    onReady
  }: {
    placeholder?: string;
    editable?: boolean;
    autofocus?: boolean;
    testid?: string;
    onUpdate?: (text: string) => void;
    onKeyDown?: (event: KeyboardEvent) => boolean;
    onPaste?: (event: ClipboardEvent) => boolean;
    onReady?: (api: TipTapEditorApi) => void;
  } = $props();

  let editorElement = $state<HTMLDivElement>();
  let editor = $state<Editor | null>(null);

  /**
   * Convert plain text to TipTap-compatible HTML.
   * Each line becomes a paragraph. Empty lines use `<p></p>` (no `<br>`):
   * the HardBreak extension's renderText returns `\n`, so a `<br>` inside an
   * empty paragraph would round-trip back through getText() as an extra
   * newline on top of the block separator, doubling blank lines on each edit.
   */
  function plainTextToHtml(text: string): string {
    if (!text) return '<p></p>';
    return text
      .split('\n')
      .map((line) => {
        if (!line) return '<p></p>';
        const escaped = line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        return `<p>${escaped}</p>`;
      })
      .join('');
  }

  function buildApi(e: Editor): TipTapEditorApi {
    return {
      getText: () => e.getText({ blockSeparator: '\n' }),

      setContent: (text: string) => {
        if (e.isDestroyed) return;
        e.commands.setContent(plainTextToHtml(text));
      },

      focus: (position: 'start' | 'end' = 'end') => {
        if (e.isDestroyed) return;
        e.commands.focus(position);
      },

      getTextBeforeCursor: () => {
        const { from } = e.state.selection;
        return e.state.doc.textBetween(0, from, '\n');
      },

      replaceTextBeforeCursor: (charCount: number, replacement: string) => {
        const { from } = e.state.selection;
        e.chain()
          .focus()
          .deleteRange({ from: from - charCount, to: from })
          .insertContent(replacement)
          .run();
      }
    };
  }

  // Create and destroy editor with the DOM element lifecycle.
  // Only editorElement should trigger recreation — all other props are
  // handled by the incremental-update effects below, so we untrack them
  // to avoid destroying/recreating the editor on prop changes.
  $effect(() => {
    if (!editorElement) return;

    const e = untrack(
      () =>
        new Editor({
          element: editorElement,
          extensions: [
            StarterKit.configure({
              // Phase 1: disable formatting extensions (plain text only)
              bold: false,
              italic: false,
              strike: false,
              code: false,
              codeBlock: false,
              blockquote: false,
              bulletList: false,
              orderedList: false,
              listItem: false,
              heading: false,
              horizontalRule: false
            }),
            Placeholder.configure({ placeholder })
          ],
          content: '<p></p>',
          editable,
          autofocus: autofocus ? 'end' : false,
          editorProps: {
            attributes: testid ? { 'data-testid': testid } : {},
            handleKeyDown: (_view, event) => {
              return onKeyDown?.(event) ?? false;
            },
            handlePaste: (_view, event) => {
              return onPaste?.(event) ?? false;
            }
          },
          onUpdate: ({ editor: ed }) => {
            onUpdate?.(ed.getText({ blockSeparator: '\n' }));
          }
        })
    );

    editor = e;

    // Notify parent that editor is ready with API
    tick().then(() => {
      if (e.isDestroyed || editor !== e) return;
      onReady?.(buildApi(e));
    });

    return () => {
      editor?.destroy();
      editor = null;
    };
  });

  // React to editable prop changes
  $effect(() => {
    if (editor && editor.isEditable !== editable) {
      editor.setEditable(editable);
    }
  });

  // React to placeholder prop changes (e.g., switching between normal and edit mode)
  $effect(() => {
    if (!editor) return;
    const ext = editor.extensionManager.extensions.find((e) => e.name === 'placeholder');
    if (ext && ext.options.placeholder !== placeholder) {
      ext.options.placeholder = placeholder;
      // Force ProseMirror to re-render decorations with the new placeholder
      editor.view.dispatch(editor.state.tr);
    }
  });
</script>

<div
  bind:this={editorElement}
  class={[
    'tiptap-editor max-h-50 min-h-8 flex-1 overflow-x-hidden overflow-y-auto bg-transparent py-1 text-text',
    !editable && 'cursor-not-allowed'
  ]}
></div>

<style>
  /* ProseMirror needs explicit outline removal and placeholder styling
	   that can't be achieved with Tailwind alone (pseudo-elements) */
  :global(.tiptap-editor .ProseMirror) {
    outline: none;
    word-break: break-word;
    font-size: 16px; /* Prevent iOS Safari auto-zoom on focus */
  }

  :global(.tiptap-editor .ProseMirror p) {
    margin: 0;
  }

  /* Placeholder styling via the Placeholder extension */
  :global(.tiptap-editor .ProseMirror p.is-editor-empty:first-child::before) {
    content: attr(data-placeholder);
    float: left;
    pointer-events: none;
    height: 0;
    color: var(--color-muted);
  }
</style>
