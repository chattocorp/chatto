<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MessageMetaBar from './MessageMetaBar.svelte';

  const { Story } = defineMeta({
    title: 'Chat/Message meta bar',
    component: MessageMetaBar,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import MessageMetaBarStoryFrame from './MessageMetaBarStoryFrame.svelte';
</script>

<Story name="Reactions" asChild>
  <MessageMetaBarStoryFrame variant="reactions" />
</Story>

<Story name="Replies and reactions" asChild>
  <MessageMetaBarStoryFrame variant="replies-and-reactions" />
</Story>

<Story name="Unread followed thread" asChild>
  <MessageMetaBarStoryFrame variant="unread-followed-thread" />
</Story>

<Story name="Thread echo" asChild>
  <MessageMetaBarStoryFrame variant="thread-echo" />
</Story>

<Story name="Read-only reactions" asChild>
  <MessageMetaBarStoryFrame variant="read-only-reactions" />
</Story>
