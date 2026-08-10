<!--
@component

Lightweight TipTap editor wrapper for chat input. Manages editor lifecycle
and exposes a typed API for text manipulation (mentions, emoji, drafts).

**Props:**
- `placeholder` - Placeholder text shown when editor is empty
- `editable` - Whether the editor accepts input
- `autofocus` - Focus editor on mount
- `testid` - data-testid attribute for E2E testing
- `onUpdate` - Called with markdown content on each change
- `onKeyDown` - Keyboard event handler; return true to prevent TipTap default
- `onPaste` - Paste event handler; return true to prevent TipTap default
- `onNextEnterWillSendChange` - Called when selection enters/leaves a trailing empty paragraph
- `onRichStructureChange` - Called when the document gains/loses structural Markdown blocks
- `onReady` - Called with editor API when editor is initialized
-->
<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { Editor } from '@tiptap/core';
  import { Slice } from '@tiptap/pm/model';
  import { CODE_LANGUAGE_OPTIONS, ensureCodeLanguagesLoaded } from '$lib/codeHighlighting';
  import { m } from '$lib/i18n/messages';
  import type { QuoteInsertionContent } from '$lib/state/room';
  import type {
    ComposerFormattingCommand,
    ComposerFormattingState,
    TipTapEditorApi
  } from './editorTypes';
  import { createComposerExtensions } from './extensions';
  import {
    applyDestinationMarks,
    buildQuoteContent,
    createClipboardContent,
    getSerializedMarkdown,
    hasDefaultEmptyDocument,
    hasRichStructure,
    isSelectionInTrailingEmptyParagraph,
    normalizeQuoteInsertionContent,
    prepareMarkdownForEditor
  } from './markdown';

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

  let {
    placeholder = m('composer.placeholder'),
    editable = true,
    autofocus = false,
    testid,
    onUpdate,
    onKeyDown,
    onPaste,
    onNextEnterWillSendChange,
    onRichStructureChange,
    onFormattingStateChange,
    onReady
  }: {
    placeholder?: string;
    editable?: boolean;
    autofocus?: boolean;
    testid?: string;
    onUpdate?: (text: string) => void;
    onKeyDown?: (event: KeyboardEvent) => boolean;
    onPaste?: (event: ClipboardEvent) => boolean;
    onNextEnterWillSendChange?: (value: boolean) => void;
    onRichStructureChange?: (value: boolean) => void;
    onFormattingStateChange?: (state: ComposerFormattingState) => void;
    onReady?: (api: TipTapEditorApi) => void;
  } = $props();

  let editorElement = $state<HTMLDivElement>();
  let editorFrameElement = $state<HTMLDivElement>();
  let editor = $state<Editor | null>(null);
  let activeCodeBlockLanguage = $state<string | null>(null);
  let activeCodeBlockSelectorPosition = $state<{ right: number; bottom: number } | null>(null);
  let activeLinkHref = $state<string | null>(null);
  let activeLinkRange = $state<{ from: number; to: number } | null>(null);
  let linkHrefDraft = $state('');
  let linkDraftInitializedFor = $state<string | null>(null);
  let codeLanguageLoadToken = 0;
  let lastNextEnterWillSend = false;
  let lastRichStructure = false;

  let hasLinkControls = $derived(activeLinkHref !== null);
  let activeCodeBlockLanguageLabel = $derived(
    CODE_LANGUAGE_OPTIONS.find((language) => language.value === activeCodeBlockLanguage)?.label ??
      activeCodeBlockLanguage?.toUpperCase() ??
      'TEXT'
  );
  let codeLanguageSelectStyle = $derived(
    activeCodeBlockSelectorPosition
      ? `right: ${activeCodeBlockSelectorPosition.right}px; bottom: ${activeCodeBlockSelectorPosition.bottom}px;`
      : ''
  );

  function getAdjacentLinkRange(e: Editor) {
    const linkType = e.state.schema.marks.link;
    const { selection } = e.state;
    if (!linkType || !selection.empty) return null;

    const fromPos = selection.$from;
    const adjacentNodes = [
      { node: fromPos.nodeBefore, from: fromPos.pos - (fromPos.nodeBefore?.nodeSize ?? 0) },
      { node: fromPos.nodeAfter, from: fromPos.pos }
    ];

    for (const { node, from } of adjacentNodes) {
      const mark = node?.marks.find((m) => m.type === linkType);
      if (node && mark) {
        return { href: mark.attrs.href ?? '', range: { from, to: from + node.nodeSize } };
      }
    }

    return null;
  }

  function updateActiveCodeBlockSelectorPosition(e: Editor) {
    if (!editorFrameElement || !e.isActive('codeBlock')) {
      activeCodeBlockSelectorPosition = null;
      return;
    }

    const selectionFrom = e.state.selection.$from;
    let codeBlockDepth = 0;
    for (let depth = selectionFrom.depth; depth > 0; depth -= 1) {
      if (selectionFrom.node(depth).type.name === 'codeBlock') {
        codeBlockDepth = depth;
        break;
      }
    }

    if (!codeBlockDepth) {
      activeCodeBlockSelectorPosition = null;
      return;
    }

    const codeBlockPosition = selectionFrom.before(codeBlockDepth);
    const nodeDom = e.view.nodeDOM(codeBlockPosition);
    const preElement =
      nodeDom instanceof HTMLElement
        ? nodeDom.tagName === 'PRE'
          ? nodeDom
          : nodeDom.closest('pre')
        : null;
    if (!preElement) {
      activeCodeBlockSelectorPosition = null;
      return;
    }

    const frameRect = editorFrameElement.getBoundingClientRect();
    const preRect = preElement.getBoundingClientRect();
    activeCodeBlockSelectorPosition = {
      right: frameRect.right - preRect.right,
      bottom: frameRect.bottom - preRect.bottom
    };
  }

  function updateNextEnterWillSend(e: Editor) {
    const value = !e.isDestroyed && isSelectionInTrailingEmptyParagraph(e);
    if (value === lastNextEnterWillSend) return;
    lastNextEnterWillSend = value;
    onNextEnterWillSendChange?.(value);
  }

  function updateRichStructure(e: Editor) {
    const value = !e.isDestroyed && hasRichStructure(e);
    if (value === lastRichStructure) return;
    lastRichStructure = value;
    onRichStructureChange?.(value);
  }

  function getFormattingState(e: Editor): ComposerFormattingState {
    if (e.isDestroyed) return emptyFormattingState;
    return {
      bold: e.isActive('bold'),
      italic: e.isActive('italic'),
      inlineCode: e.isActive('code'),
      heading: e.isActive('heading', { level: 2 }),
      bulletList: e.isActive('bulletList'),
      orderedList: e.isActive('orderedList'),
      blockquote: e.isActive('blockquote'),
      codeBlock: e.isActive('codeBlock')
    };
  }

  function updateActiveControls(e: Editor) {
    onFormattingStateChange?.(getFormattingState(e));

    if (e.isActive('codeBlock')) {
      activeCodeBlockLanguage = e.getAttributes('codeBlock').language || 'text';
    } else {
      activeCodeBlockLanguage = null;
    }
    updateActiveCodeBlockSelectorPosition(e);

    const adjacentLink = getAdjacentLinkRange(e);

    if (e.isActive('link') || adjacentLink) {
      const href = adjacentLink?.href ?? e.getAttributes('link').href ?? '';
      activeLinkHref = href;
      activeLinkRange = adjacentLink?.range ?? null;
      if (linkDraftInitializedFor !== href) {
        linkHrefDraft = href;
        linkDraftInitializedFor = href;
      }
    } else {
      activeLinkHref = null;
      activeLinkRange = null;
      linkHrefDraft = '';
      linkDraftInitializedFor = null;
    }
  }

  function setCodeBlockLanguage(language: string) {
    if (!editor) return;

    editor
      .chain()
      .focus()
      .updateAttributes('codeBlock', { language: language || null })
      .run();
    updateActiveControls(editor);
    ensureEditorCodeLanguages(editor);
  }

  function getEditorCodeBlockLanguages(e: Editor): string[] {
    const languages: string[] = [];

    e.state.doc.descendants((node) => {
      if (node.type.name === 'codeBlock') {
        const language = node.attrs.language || 'text';
        if (!languages.includes(language)) {
          languages.push(language);
        }
      }
    });

    return languages;
  }

  function refreshCodeBlockDecorations(e: Editor) {
    if (e.isDestroyed) return;

    let tr = e.state.tr;
    e.state.doc.descendants((node, pos) => {
      if (node.type.name === 'codeBlock') {
        tr = tr.setNodeMarkup(pos, undefined, node.attrs, node.marks);
      }
    });

    if (tr.steps.length > 0) {
      e.view.dispatch(tr);
    }
  }

  function ensureEditorCodeLanguages(e: Editor) {
    const languages = getEditorCodeBlockLanguages(e);
    if (languages.length === 0) return;

    const loadToken = ++codeLanguageLoadToken;
    ensureCodeLanguagesLoaded(languages).then((loadedNewLanguage) => {
      if (
        !loadedNewLanguage ||
        e.isDestroyed ||
        editor !== e ||
        loadToken !== codeLanguageLoadToken
      ) {
        return;
      }

      refreshCodeBlockDecorations(e);
      updateActiveControls(e);
    });
  }

  function normalizeHref(href: string) {
    const trimmed = href.trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed;
    return `https://${trimmed}`;
  }

  function applyLinkHref() {
    if (!editor || activeLinkHref === null) return;

    const href = normalizeHref(linkHrefDraft);
    if (!href) {
      removeLink();
      return;
    }

    const linkType = editor.state.schema.marks.link;
    if (activeLinkRange && linkType) {
      const tr = editor.state.tr.addMark(
        activeLinkRange.from,
        activeLinkRange.to,
        linkType.create({ href })
      );
      editor.view.dispatch(tr);
      editor.commands.focus();
    } else {
      editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
    }
    updateActiveControls(editor);
  }

  function removeLink() {
    if (!editor) return;

    const linkType = editor.state.schema.marks.link;
    if (activeLinkRange && linkType) {
      const tr = editor.state.tr.removeMark(activeLinkRange.from, activeLinkRange.to, linkType);
      editor.view.dispatch(tr);
      editor.commands.focus();
    } else {
      editor.chain().focus().extendMarkRange('link').unsetLink().run();
    }
    updateActiveControls(editor);
  }

  function openActiveLink() {
    const href = normalizeHref(activeLinkHref ?? '');
    if (!href) return;

    window.open(href, '_blank', 'noopener,noreferrer');
  }

  function buildApi(e: Editor): TipTapEditorApi {
    const syncControls = () => {
      if (e.isDestroyed) return;
      updateActiveControls(e);
      updateNextEnterWillSend(e);
      updateRichStructure(e);
    };

    return {
      getText: () => (e.isDestroyed ? '' : e.getText({ blockSeparator: '\n' })),

      setContent: (markdown: string) => {
        if (e.isDestroyed) return;
        e.commands.setContent(prepareMarkdownForEditor(markdown), {
          contentType: 'markdown',
          emitUpdate: false
        });
        updateRichStructure(e);
        ensureEditorCodeLanguages(e);
        tick().then(syncControls);
      },

      focus: (position: 'start' | 'end' = 'end') => {
        if (e.isDestroyed) return;
        e.commands.focus(position);
        tick().then(syncControls);
      },

      getTextBeforeCursor: () => {
        if (e.isDestroyed) return '';
        const { from } = e.state.selection;
        return e.state.doc.textBetween(0, from, '\n');
      },

      isInCodeBlock: () => !e.isDestroyed && e.isActive('codeBlock'),

      replaceTextBeforeCursor: (charCount: number, replacement: string) => {
        if (e.isDestroyed) return;
        const { from } = e.state.selection;
        e.chain()
          .focus()
          .deleteRange({ from: from - charCount, to: from })
          .insertContent(replacement)
          .run();
        tick().then(syncControls);
      },

      insertText: (text: string) => {
        if (e.isDestroyed || !text) return;
        e.chain().focus().insertContent(text).run();
        tick().then(syncControls);
      },

      toggleFormatting: (command: ComposerFormattingCommand) => {
        if (e.isDestroyed) return;

        const chain = e.chain().focus();
        switch (command) {
          case 'bold':
            chain.toggleBold().run();
            break;
          case 'italic':
            chain.toggleItalic().run();
            break;
          case 'inlineCode':
            chain.toggleCode().run();
            break;
          case 'heading':
            chain.toggleHeading({ level: 2 }).run();
            break;
          case 'bulletList':
            chain.toggleBulletList().run();
            break;
          case 'orderedList':
            chain.toggleOrderedList().run();
            break;
          case 'blockquote':
            chain.toggleBlockquote().run();
            break;
          case 'codeBlock':
            chain.toggleCodeBlock().run();
            ensureEditorCodeLanguages(e);
            break;
        }
        tick().then(syncControls);
      },

      insertQuote: (text: QuoteInsertionContent) => {
        if (e.isDestroyed) return;
        const quoteBlocks = normalizeQuoteInsertionContent(text);
        if (quoteBlocks.length === 0) return;
        const quoteContent = buildQuoteContent(quoteBlocks);

        e.chain()
          .focus()
          .insertContent([
            {
              type: 'blockquote',
              content: quoteContent
            },
            { type: 'paragraph' }
          ])
          .run();
        tick().then(syncControls);
      },

      insertBlockBreak: () => {
        if (e.isDestroyed) return;
        e.chain().focus().splitBlock().run();
        tick().then(syncControls);
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
          extensions: createComposerExtensions(placeholder),
          content: '',
          contentType: 'markdown',
          editable,
          autofocus: autofocus ? 'end' : false,
          editorProps: {
            attributes: {
              'aria-label': placeholder,
              ...(testid ? { 'data-testid': testid } : {})
            },
            handleKeyDown: (_view, event) => {
              return onKeyDown?.(event) ?? false;
            },
            clipboardTextParser: (text, context, _plain, view) => {
              const normalizedText = text.replace(/\r\n?/g, '\n');
              const markdown = editor?.markdown;
              const destinationMarks = view.state.storedMarks ?? context.marks();
              const document = markdown
                ? view.state.schema.nodeFromJSON(
                    markdown.parse(prepareMarkdownForEditor(normalizedText))
                  )
                : null;
              const content =
                document && document.content.size > 0
                  ? applyDestinationMarks(document, destinationMarks).content
                  : createClipboardContent(normalizedText, view.state.schema, destinationMarks);

              return Slice.maxOpen(content);
            },
            handlePaste: (view, event) => {
              if (onPaste?.(event)) return true;

              const text = event.clipboardData?.getData('text/plain');
              const html = event.clipboardData?.getData('text/html');
              if (!text || !html || editor?.isActive('codeBlock')) return false;

              // Prefer and Markdown-parse the textual representation when the clipboard also
              // supplies HTML.
              view.pasteText(text);
              return true;
            }
          },
          onUpdate: ({ editor: ed }) => {
            updateActiveControls(ed);
            updateNextEnterWillSend(ed);
            updateRichStructure(ed);
            ensureEditorCodeLanguages(ed);
            onUpdate?.(hasDefaultEmptyDocument(ed) ? '' : getSerializedMarkdown(ed));
          },
          onSelectionUpdate: ({ editor: ed }) => {
            updateActiveControls(ed);
            updateNextEnterWillSend(ed);
            updateRichStructure(ed);
          }
        })
    );

    editor = e;

    // Notify parent that editor is ready with API
    tick().then(() => {
      if (e.isDestroyed || editor !== e) return;
      updateActiveControls(e);
      updateNextEnterWillSend(e);
      updateRichStructure(e);
      onReady?.(buildApi(e));
    });

    return () => {
      lastNextEnterWillSend = false;
      onNextEnterWillSendChange?.(false);
      lastRichStructure = false;
      onRichStructureChange?.(false);
      onFormattingStateChange?.(emptyFormattingState);
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
    if (editor.view.dom.getAttribute('aria-label') !== placeholder) {
      editor.view.dom.setAttribute('aria-label', placeholder);
    }
    const ext = editor.extensionManager.extensions.find((e) => e.name === 'placeholder');
    if (ext && ext.options.placeholder !== placeholder) {
      ext.options.placeholder = placeholder;
      // Force ProseMirror to re-render decorations with the new placeholder
      editor.view.dispatch(editor.state.tr);
    }
  });
</script>

<svelte:window onresize={() => editor && updateActiveControls(editor)} />

<div bind:this={editorFrameElement} class="relative flex min-w-0 flex-1 flex-col gap-1">
  {#if hasLinkControls}
    <div class="flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted">
      <div class="flex min-w-0 items-center gap-1">
        <input
          name="composer-link-url"
          aria-label={m('composer.link_url')}
          title={m('composer.link_url')}
          value={linkHrefDraft}
          disabled={!editable}
          oninput={(event) => (linkHrefDraft = event.currentTarget.value)}
          onkeydown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              applyLinkHref();
            }
          }}
          onblur={applyLinkHref}
          class="h-10 w-48 min-w-0 rounded border border-border bg-surface-emphasized px-2 text-xs text-text transition-[background-color,border-color] outline-none hover:bg-surface-strong focus:border-action disabled:cursor-not-allowed disabled:opacity-50"
        />
        <button
          type="button"
          aria-label={m('composer.open_link')}
          title={m('composer.open_link')}
          disabled={!activeLinkHref}
          onclick={openActiveLink}
          class="flex h-10 w-10 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface-strong hover:text-text active:scale-[0.96]"
        >
          <span class="iconify icon-[uil--external-link-alt] text-base"></span>
        </button>
        <button
          type="button"
          aria-label={m('composer.remove_link')}
          title={m('composer.remove_link')}
          disabled={!editable}
          onclick={removeLink}
          class="flex h-10 w-10 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface-strong hover:text-text active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span class="iconify icon-[uil--link-broken] text-base"></span>
        </button>
      </div>
    </div>
  {/if}

  <div
    bind:this={editorElement}
    onscroll={() => editor && updateActiveControls(editor)}
    class={[
      'tiptap-editor max-h-50 min-h-8 min-w-0 flex-1 overflow-x-hidden overflow-y-auto bg-transparent py-1 text-text',
      !editable && 'cursor-not-allowed'
    ]}
  ></div>

  {#if activeCodeBlockLanguage !== null && activeCodeBlockSelectorPosition}
    <div class="absolute z-10" style={codeLanguageSelectStyle}>
      <div
        class="group relative inline-flex h-6 items-center gap-1 rounded-tl-md rounded-br-md bg-surface-emphasized pr-1.5 pl-2 font-mono text-xs tracking-wide text-muted uppercase focus-within:bg-surface-strong focus-within:text-text focus-within:ring-1 focus-within:ring-action hover:bg-surface-strong hover:text-text"
      >
        <span>{activeCodeBlockLanguageLabel}</span>
        <span class="iconify icon-[uil--angle-down] size-3"></span>
        <select
          name="composer-code-language"
          aria-label={m('composer.code_language')}
          title={m('composer.code_language')}
          value={activeCodeBlockLanguage}
          disabled={!editable}
          onchange={(event) => setCodeBlockLanguage(event.currentTarget.value)}
          class="absolute inset-0 h-full w-full cursor-pointer opacity-0 disabled:cursor-not-allowed"
        >
          {#each CODE_LANGUAGE_OPTIONS as language (language.value)}
            <option value={language.value}>{language.label}</option>
          {/each}
        </select>
      </div>
    </div>
  {/if}
</div>

<style>
  /* ProseMirror needs explicit outline removal and placeholder styling
	   that can't be achieved with Tailwind alone (pseudo-elements) */
  :global(.tiptap-editor .ProseMirror) {
    outline: none;
    word-break: break-word;
    font-size: 16px; /* Prevent iOS Safari auto-zoom on focus */
    line-height: 1.5;
    text-align: start;
  }

  :global(.tiptap-editor .ProseMirror p),
  :global(.tiptap-editor .ProseMirror blockquote),
  :global(.tiptap-editor .ProseMirror ul),
  :global(.tiptap-editor .ProseMirror ol),
  :global(.tiptap-editor .ProseMirror pre),
  :global(.tiptap-editor .ProseMirror h1),
  :global(.tiptap-editor .ProseMirror h2),
  :global(.tiptap-editor .ProseMirror h3),
  :global(.tiptap-editor .ProseMirror h4),
  :global(.tiptap-editor .ProseMirror h5),
  :global(.tiptap-editor .ProseMirror h6) {
    margin: 0;
    /* Stable wrapping is essential while editing: `pretty` and `balance`
       may move an earlier line break whenever the document changes. */
    text-wrap: wrap;
  }

  :global(.tiptap-editor .ProseMirror > p),
  :global(.tiptap-editor .ProseMirror > blockquote),
  :global(.tiptap-editor .ProseMirror > ul),
  :global(.tiptap-editor .ProseMirror > ol),
  :global(.tiptap-editor .ProseMirror > h1),
  :global(.tiptap-editor .ProseMirror > h2),
  :global(.tiptap-editor .ProseMirror > h3),
  :global(.tiptap-editor .ProseMirror > h4),
  :global(.tiptap-editor .ProseMirror > h5),
  :global(.tiptap-editor .ProseMirror > h6) {
    margin-block: 0.5em;
  }

  :global(.tiptap-editor .ProseMirror > :first-child) {
    margin-top: 0;
  }

  :global(.tiptap-editor .ProseMirror > :last-child) {
    margin-bottom: 0;
  }

  :global(.tiptap-editor .ProseMirror li > p) {
    margin: 0;
  }

  :global(.tiptap-editor .ProseMirror strong) {
    font-weight: 600;
  }

  :global(.tiptap-editor .ProseMirror a) {
    color: var(--color-link);
    text-decoration: underline;
    text-underline-offset: 2px;
    overflow-wrap: anywhere;
  }

  :global(.tiptap-editor .ProseMirror ul),
  :global(.tiptap-editor .ProseMirror ol) {
    padding-inline-start: 1.5em;
  }

  :global(.tiptap-editor .ProseMirror ul) {
    list-style-type: disc;
  }

  :global(.tiptap-editor .ProseMirror ol) {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    column-gap: 0.4em;
    padding-inline-start: 0;
    list-style: none;
  }

  :global(.tiptap-editor .ProseMirror ol > li) {
    counter-increment: list-item;
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
  }

  :global(.tiptap-editor .ProseMirror ol > li::before) {
    content: counter(list-item) '.';
    grid-column: 1;
    text-align: end;
  }

  :global(.tiptap-editor .ProseMirror ol > li > *) {
    grid-column: 2;
  }

  :global(.tiptap-editor .ProseMirror blockquote) {
    --composer-quote-border: color-mix(in srgb, var(--color-muted), var(--color-action) 42%);
    --composer-quote-text: color-mix(in srgb, var(--color-text), var(--color-muted) 48%);

    border-inline-start: 3px solid var(--composer-quote-border);
    padding-block: 0.35em;
    padding-inline-start: 0.9em;
    color: var(--composer-quote-text);
    font-style: italic;
  }

  :global(.tiptap-editor .ProseMirror blockquote > *) {
    margin-block: 0;
  }

  :global(.tiptap-editor .ProseMirror blockquote > * + *) {
    margin-top: 0.45em;
  }

  :global(.tiptap-editor .ProseMirror code:not(pre code)) {
    border-radius: 0.25rem;
    background: var(--color-surface-emphasized);
    padding: 0.125rem 0.375rem;
    font-family: var(--font-mono);
    font-size: 0.9em;
    direction: ltr;
    unicode-bidi: isolate;
  }

  :global(.tiptap-editor .ProseMirror pre) {
    overflow: hidden;
    position: relative;
    width: 100%;
    border-radius: 0.375rem;
    border: 1px solid var(--color-surface-emphasized);
    background: transparent;
    padding: 0.5rem 0.75rem;
    font-family: var(--font-mono);
    font-size: 0.875rem;
    line-height: 1.5;
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
    direction: ltr;
    unicode-bidi: isolate;
  }

  :global(.tiptap-editor .ProseMirror p),
  :global(.tiptap-editor .ProseMirror li),
  :global(.tiptap-editor .ProseMirror blockquote),
  :global(.tiptap-editor .ProseMirror h1),
  :global(.tiptap-editor .ProseMirror h2),
  :global(.tiptap-editor .ProseMirror h3),
  :global(.tiptap-editor .ProseMirror h4),
  :global(.tiptap-editor .ProseMirror h5),
  :global(.tiptap-editor .ProseMirror h6) {
    unicode-bidi: plaintext;
  }

  :global(.tiptap-editor .ProseMirror > pre) {
    margin-block: 0.5rem;
  }

  :global(.tiptap-editor .ProseMirror > pre:first-child) {
    margin-top: 0;
  }

  :global(.tiptap-editor .ProseMirror > pre:last-child) {
    margin-bottom: 0;
  }

  :global(.tiptap-editor .ProseMirror pre[data-language]::after) {
    content: attr(data-language);
    position: absolute;
    right: 0;
    bottom: 0;
    border-top-left-radius: 0.375rem;
    background: var(--color-surface-emphasized);
    padding: 0.125rem 0.5rem;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    line-height: 1rem;
    letter-spacing: 0.025em;
    color: var(--color-muted);
    text-transform: uppercase;
    pointer-events: none;
  }

  :global(.tiptap-editor .ProseMirror pre code) {
    display: block;
    overflow-x: auto;
    background: transparent;
    padding: 0 3.5rem 0 0;
    font-size: inherit;
    line-height: inherit;
    color: var(--composer-code-text);
    white-space: pre;
  }

  :global(.tiptap-editor) {
    --composer-code-text: #24292f;
    --composer-code-comment: #6e7781;
    --composer-code-keyword: #cf222e;
    --composer-code-string: #0a3069;
    --composer-code-title: #8250df;
    --composer-code-literal: #0550ae;
    --composer-code-attribute: #953800;
  }

  :global(:root[data-theme='dark'] .tiptap-editor) {
    --composer-code-text: #d0d7de;
    --composer-code-comment: #8b949e;
    --composer-code-keyword: #ff7b72;
    --composer-code-string: #a5d6ff;
    --composer-code-title: #d2a8ff;
    --composer-code-literal: #79c0ff;
    --composer-code-attribute: #ffa657;
  }

  :global(.tiptap-editor .ProseMirror .hljs-comment),
  :global(.tiptap-editor .ProseMirror .hljs-quote) {
    color: var(--composer-code-comment);
    font-style: italic;
  }

  :global(.tiptap-editor .ProseMirror .hljs-keyword),
  :global(.tiptap-editor .ProseMirror .hljs-selector-tag),
  :global(.tiptap-editor .ProseMirror .hljs-subst) {
    color: var(--composer-code-keyword);
  }

  :global(.tiptap-editor .ProseMirror .hljs-string),
  :global(.tiptap-editor .ProseMirror .hljs-regexp),
  :global(.tiptap-editor .ProseMirror .hljs-symbol),
  :global(.tiptap-editor .ProseMirror .hljs-bullet) {
    color: var(--composer-code-string);
  }

  :global(.tiptap-editor .ProseMirror .hljs-title),
  :global(.tiptap-editor .ProseMirror .hljs-section),
  :global(.tiptap-editor .ProseMirror .hljs-name),
  :global(.tiptap-editor .ProseMirror .hljs-selector-id),
  :global(.tiptap-editor .ProseMirror .hljs-selector-class) {
    color: var(--composer-code-title);
  }

  :global(.tiptap-editor .ProseMirror .hljs-number),
  :global(.tiptap-editor .ProseMirror .hljs-literal),
  :global(.tiptap-editor .ProseMirror .hljs-type),
  :global(.tiptap-editor .ProseMirror .hljs-built_in) {
    color: var(--composer-code-literal);
  }

  :global(.tiptap-editor .ProseMirror .hljs-attr),
  :global(.tiptap-editor .ProseMirror .hljs-attribute),
  :global(.tiptap-editor .ProseMirror .hljs-variable),
  :global(.tiptap-editor .ProseMirror .hljs-template-variable) {
    color: var(--composer-code-attribute);
  }

  :global(.tiptap-editor .ProseMirror h1),
  :global(.tiptap-editor .ProseMirror h2),
  :global(.tiptap-editor .ProseMirror h3),
  :global(.tiptap-editor .ProseMirror h4),
  :global(.tiptap-editor .ProseMirror h5),
  :global(.tiptap-editor .ProseMirror h6) {
    font-size: inherit;
    font-weight: 600;
    text-decoration: underline;
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
