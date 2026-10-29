<!-- @component Profile bio field using the shared Markdown composer field. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { ComposerEditorKind } from '$lib/state/userPreferences.svelte';
  import MarkdownEditorField from '$lib/components/composer/MarkdownEditorField.svelte';

  let {
    value = $bindable(''),
    editorKind,
    disabled = false,
    maxlength = 1000,
    placeholder,
    description,
    oninput
  }: {
    value?: string;
    editorKind: ComposerEditorKind;
    disabled?: boolean;
    maxlength?: number;
    placeholder?: string;
    description?: string;
    oninput?: () => void;
  } = $props();
</script>

<MarkdownEditorField
  bind:value
  {editorKind}
  {disabled}
  {maxlength}
  trimWhitespaceForCount
  {oninput}
  label={m('settings.profile.bio.label')}
  testid="settings-bio"
  placeholder={placeholder ?? m('settings.profile.bio.placeholder')}
  description={description ?? m('settings.profile.bio.description', { max: maxlength })}
  tooLongMessage={m('settings.profile.bio.too_long', { max: maxlength })}
/>
