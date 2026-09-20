<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ComposerToolbar from './ComposerToolbar.svelte';

  const { Story } = defineMeta({ title: 'Composer/Action toolbar', component: ComposerToolbar });
</script>

<script lang="ts">
  let createThread = $state(false);
</script>

<Story name="Narrow actions" asChild>
  <div class="@container/composer w-80 chat-input-surface p-2">
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

<Story name="Very narrow thread reply" asChild>
  <div class="@container/composer w-[180px] chat-input-surface p-2">
    <ComposerToolbar
      editorApi={null}
      inputDisabled={false}
      canAttach
      isEditing={false}
      canSubmit={false}
      showAlsoSendToChannel
      alsoSendToChannel
      onsubmit={() => {}}
    />
  </div>
</Story>

<Story name="With labels" asChild>
  <div class="@container/composer flex w-[640px] justify-end chat-input-surface p-2">
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
