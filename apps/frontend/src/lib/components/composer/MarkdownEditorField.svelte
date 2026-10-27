<!--
@component

Edits Markdown with the selected composer and its formatting toolbar. The parent
owns the draft and save operation. Editor changes preserve the current draft;
external value changes, such as a successful save, update the mounted editor.
-->
<script lang="ts">
  import type { Component } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { LoadingFog } from '$lib/ui';
  import type { ComposerEditorKind } from '$lib/state/userPreferences.svelte';
  import ComposerFormattingToolbar from '$lib/components/composer/ComposerFormattingToolbar.svelte';
  import type {
    ComposerEditorApi,
    ComposerEditorProps,
    ComposerFormattingState
  } from '$lib/components/composer/editorTypes';
  import { emptyComposerIndentState } from '$lib/components/composer/editorTypes';
  import { AutocompleteState } from './autocomplete.svelte';
  import MentionAutocomplete from './MentionAutocomplete.svelte';
  import type { MarkdownMentionOptions } from './mentionOptions';
  import type { RoomMember } from '$lib/state/room';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';

  let {
    value = $bindable(''),
    editorKind,
    disabled = false,
    maxlength = 1000,
    label,
    placeholder = '',
    description,
    testid,
    tooLongMessage,
    trimWhitespaceForCount = false,
    oninput,
    mentions
  }: {
    value?: string;
    editorKind: ComposerEditorKind;
    disabled?: boolean;
    /** Maximum number of Unicode characters in the saved Markdown source. */
    maxlength?: number;
    /** Accessible field label. */
    label: string;
    testid?: string;
    /** Optional validation message when the source exceeds maxlength. */
    tooLongMessage?: string;
    /** Use only when the save operation trims the source before validation. */
    trimWhitespaceForCount?: boolean;
    /** Editor placeholder and accessible editor name. */
    placeholder?: string;
    /** Help text under the editor. */
    description?: string;
    oninput?: () => void;
    /** Enables the chat mention picker without sending mention notifications. */
    mentions?: MarkdownMentionOptions;
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
  const characterCount = $derived([...(trimWhitespaceForCount ? value.trim() : value)].length);
  let editorApi = $state.raw<ComposerEditorApi | null>(null);
  let lastEditorValue = '';
  let searchResults = $state.raw<{ source: MarkdownMentionOptions; members: RoomMember[] } | null>(
    null
  );
  const searchDebounce = useDebounce();
  let searchGeneration = 0;
  const candidates = $derived([
    ...(mentions?.members ?? []),
    ...(searchResults?.source === mentions ? (searchResults?.members ?? []) : []).filter(
      (result) => !mentions?.members.some((member) => member.id === result.id)
    )
  ]);
  const autocomplete = new AutocompleteState(
    () => editorApi,
    () => candidates,
    () => mentions?.roles ?? [],
    undefined,
    { openOnAt: true, emojis: false }
  );

  function resetAutocomplete() {
    searchGeneration++;
    searchDebounce.cancel();
    searchResults = null;
    autocomplete.reset();
  }

  function updateMentions() {
    if (!mentions || disabled) return;
    autocomplete.update();
    const generation = ++searchGeneration;
    searchDebounce.cancel();
    searchResults = null;
    const query = autocomplete.mention?.query;
    const source = mentions;
    const search = source.searchMembers;
    if (!query || !search) return;
    searchDebounce.run(() => {
      void search(query)
        .then((results) => {
          if (generation === searchGeneration && source === mentions && editorApi)
            searchResults = { source, members: results };
        })
        .catch(() => {
          // Loaded members remain available if directory search fails.
        });
    }, 150);
  }

  function handleKeyDown(event: KeyboardEvent): boolean {
    if (!mentions || disabled || event.isComposing) return false;
    if (autocomplete.mention && autocomplete.mentionRef?.handleKeyDown(event)) return true;
    if (event.key === 'Tab' && autocomplete.handleTabCompletion(event)) return true;
    if (event.key !== 'Tab') autocomplete.resetTabCompletion();
    return false;
  }
  let formattingState = $state<ComposerFormattingState>({
    bold: false,
    italic: false,
    underline: false,
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
    updateMentions();
    oninput?.();
  }

  // Synchronize external draft changes with the imperative editor API. Local
  // editor updates already contain this value and must not reset the selection.
  // Replacing the document also invalidates completion offsets and searches.
  $effect(() => {
    if (editorApi && value !== lastEditorValue) {
      resetAutocomplete();
      lastEditorValue = value;
      editorApi.setContent(value);
    }
  });
</script>

<fieldset class="flex min-w-0 flex-col gap-1.5" aria-describedby={id + '-description'}>
  <legend class="mb-1.5 text-sm font-medium text-text">{label}</legend>
  <ComposerFormattingToolbar
    id={id + '-formatting'}
    {formattingState}
    {indentState}
    {editorApi}
    inputDisabled={disabled}
    animated={false}
  />
  <div class="relative input min-w-0 [--composer-min-height:8rem] [&_.ProseMirror]:min-h-32">
    {#if mentions && !disabled && autocomplete.mention}
      <MentionAutocomplete
        bind:this={autocomplete.mentionRef}
        query={autocomplete.mention.query}
        members={candidates}
        roles={mentions.roles}
        onSelect={(handle, viaTab) => autocomplete.selectMention(handle, viaTab)}
        onClose={resetAutocomplete}
      />
    {/if}
    {#key editorKind}
      {#await editorModule}
        <LoadingFog class="h-32 w-full" />
      {:then { default: Editor }}
        <Editor
          {placeholder}
          editable={!disabled}
          {testid}
          onReady={ready}
          onDestroy={(api) => {
            if (editorApi === api) {
              editorApi = null;
              resetAutocomplete();
            }
          }}
          onUpdate={update}
          onKeyDown={handleKeyDown}
          onFormattingStateChange={(state) => (formattingState = state)}
          onIndentStateChange={(state) => (indentState = state)}
        />
      {:catch}
        <p role="alert">{m('common.error.generic')}</p>
      {/await}
    {/key}
  </div>
  <p id={id + '-description'} class="text-sm text-muted">
    {description}
  </p>
  <p class={['text-end text-sm', characterCount > maxlength ? 'text-danger' : 'text-muted']}>
    {characterCount} / {maxlength}
  </p>
  {#if characterCount > maxlength && tooLongMessage}
    <p role="alert" class="text-sm text-danger">
      {tooLongMessage}
    </p>
  {/if}
</fieldset>
