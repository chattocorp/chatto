<!--
@component

Edits a profile bio as Markdown with the preferred message editor. The parent
owns the draft and save operation. Editor changes preserve the current draft;
external value changes, such as a successful save, update the mounted editor.
-->
<script lang="ts">
  import type { Component } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import type { ComposerEditorKind } from '$lib/state/userPreferences.svelte';
  import ComposerFormattingToolbar from '$lib/components/composer/ComposerFormattingToolbar.svelte';
  import type {
    ComposerEditorApi,
    ComposerEditorProps,
    ComposerFormattingState
  } from '$lib/components/composer/editorTypes';
  import { emptyComposerIndentState } from '$lib/components/composer/editorTypes';

  let {
    value = $bindable(''),
    editorKind,
    disabled = false,
    maxlength = 1000,
    oninput
  }: {
    value?: string;
    editorKind: ComposerEditorKind;
    disabled?: boolean;
    /** Maximum number of Unicode characters in the saved Markdown source. */
    maxlength?: number;
    oninput?: () => void;
  } = $props();

  const loaders: Record<
    ComposerEditorKind,
    () => Promise<{ default: Component<ComposerEditorProps> }>
  > = {
    visual: () => import('$lib/components/composer/TipTapEditor.svelte'),
    markdown: () => import('$lib/components/composer/MarkdownEditor.svelte')
  };
  const editorModule = $derived(loaders[editorKind]());
  const id = $props.id();
  const characterCount = $derived([...value.trim()].length);
  let editorApi = $state.raw<ComposerEditorApi | null>(null);
  let lastEditorValue = '';
  let formattingState = $state<ComposerFormattingState>({
    bold: false,
    italic: false,
    inlineCode: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    blockquote: false,
    codeBlock: false
  });
  let indentState = $state(emptyComposerIndentState);

  function ready(api: ComposerEditorApi) {
    lastEditorValue = value;
    api.setContent(value);
    editorApi = api;
  }

  function update(markdown: string) {
    lastEditorValue = markdown;
    value = markdown;
    oninput?.();
  }

  // Synchronize external draft changes with the imperative editor API. Local
  // editor updates already contain this value and must not reset the selection.
  $effect(() => {
    if (editorApi && value !== lastEditorValue) {
      lastEditorValue = value;
      editorApi.setContent(value);
    }
  });
</script>

<fieldset class="flex min-w-0 flex-col gap-1.5" aria-describedby={id + '-description'}>
  <legend class="mb-1.5 text-sm font-medium text-text">{m('settings.profile.bio.label')}</legend>
  <ComposerFormattingToolbar
    id={id + '-formatting'}
    {formattingState}
    {indentState}
    {editorApi}
    inputDisabled={disabled}
    animated={false}
  />
  <div class="input min-w-0 [--composer-min-height:8rem] [&_.ProseMirror]:min-h-32">
    {#key editorKind}
      {#await editorModule}
        <LoadingFog class="h-32 w-full" />
      {:then { default: Editor }}
        <Editor
          placeholder={m('settings.profile.bio.placeholder')}
          editable={!disabled}
          testid="settings-bio"
          onReady={ready}
          onDestroy={(api) => {
            if (editorApi === api) editorApi = null;
          }}
          onUpdate={update}
          onFormattingStateChange={(state) => (formattingState = state)}
          onIndentStateChange={(state) => (indentState = state)}
        />
      {:catch}
        <p role="alert">{m('common.error.generic')}</p>
      {/await}
    {/key}
  </div>
  <p id={id + '-description'} class="text-sm text-muted">
    {m('settings.profile.bio.description', { max: maxlength })}
  </p>
  <p class={['text-end text-sm', characterCount > maxlength ? 'text-danger' : 'text-muted']}>
    {characterCount} / {maxlength}
  </p>
  {#if characterCount > maxlength}
    <p role="alert" class="text-sm text-danger">
      {m('settings.profile.bio.too_long', { max: maxlength })}
    </p>
  {/if}
</fieldset>
