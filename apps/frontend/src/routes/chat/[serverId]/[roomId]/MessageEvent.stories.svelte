<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Chat/Message rows',
    parameters: {
      layout: 'fullscreen'
    }
  });
</script>

<script lang="ts">
  import MessageRowStoryFrame from './MessageRowStoryFrame.svelte';
</script>

<Story name="Plain message" asChild>
  <MessageRowStoryFrame variant="plain" />
</Story>

<Story name="Message with meta bar" asChild>
  <MessageRowStoryFrame variant="with-meta-bar" />
</Story>

<Story name="Footer comparison" asChild>
  <MessageRowStoryFrame variant="footer-comparison" />
</Story>

<Story name="Compact grouped message" asChild>
  <MessageRowStoryFrame variant="compact-grouped" />
</Story>

<Story name="Search result" asChild>
  <MessageRowStoryFrame variant="search-result" />
</Story>

<Story name="Deleted message" asChild>
  <MessageRowStoryFrame variant="deleted" />
</Story>
