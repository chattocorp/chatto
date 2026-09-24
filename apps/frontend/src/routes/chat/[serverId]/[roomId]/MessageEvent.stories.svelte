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
  import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
  import { UserStore } from '$lib/state/server/users.svelte';
  import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
  import MessageRowStoryFrame from './MessageRowStoryFrame.svelte';
  import MessageEventTestHarness from './MessageEventTestHarness.svelte';

  const lateAuthor = new UserStore();
  const unresolvedMessage: TimelineEventView = {
    id: 'late-author-message',
    actorId: 'author',
    actor: null,
    actorResolution: 'unavailable',
    createdAt: '2026-09-24T10:00:00Z',
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'My profile arrived after this message.',
      attachments: [],
      reactions: [],
      replyCount: 0,
      threadParticipants: []
    }
  };
</script>

<Story name="Plain message" asChild>
  <MessageRowStoryFrame variant="plain" />
</Story>

<Story name="Mobile video attachment" asChild>
  <MessageRowStoryFrame variant="mobile-video" />
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

<Story name="Late author profile" asChild>
  <div class="min-h-screen space-y-4 bg-background p-10 text-text">
    <button
      type="button"
      class="btn"
      onclick={() => lateAuthor.set('author', new DirectoryMember({
        user: { id: 'author', login: 'author', displayName: 'Resolved author' }
      }))}
    >Load author profile</button>
    <MessageEventTestHarness event={unresolvedMessage} userStore={lateAuthor} />
  </div>
</Story>
