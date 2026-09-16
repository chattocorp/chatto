<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ComposerToolbar from './ComposerToolbar.svelte';

  const { Story } = defineMeta({ title: 'Composer/Action toolbar', component: ComposerToolbar });
</script>

<script lang="ts">
  let createThread = $state(false);
</script>

<Story name="Compact glossy actions" asChild>
  <div class="@container flex w-80 justify-end chat-input-surface p-2">
    <ComposerToolbar
      editorApi={null}
      inputDisabled={false}
      canAttach
      isEditing={false}
      canSubmit
      showCreateThread
      {createThread}
      onToggleCreateThread={() => (createThread = !createThread)}
      onsubmit={() => {}}
    />
  </div>
</Story>

<Story name="With labels" asChild>
  <div class="@container flex w-[640px] justify-end chat-input-surface p-2">
    <ComposerToolbar
      editorApi={null}
      inputDisabled={false}
      canAttach
      isEditing={false}
      canSubmit
      showCreateThread
      {createThread}
      onToggleCreateThread={() => (createThread = !createThread)}
      onsubmit={() => {}}
    />
  </div>
</Story>
