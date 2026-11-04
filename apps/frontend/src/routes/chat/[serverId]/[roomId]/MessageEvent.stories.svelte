<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { messageEvent } from './messageEventFixture';

  const { Story } = defineMeta({
    title: 'Chat/Message rows',
    parameters: {
      layout: 'fullscreen'
    }
  });
</script>

<script lang="ts">
  import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
  import { UserStore } from '@chatto/client/server/users';
  import {
    TimelineEventKind,
    type TimelineEventView
  } from '@chatto/client/timeline/timelineEvents';
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
      onclick={() =>
        lateAuthor.set(
          'author',
          new DirectoryMember({
            user: { id: 'author', login: 'author', displayName: 'Resolved author' }
          })
        )}>Load author profile</button
    >
    <MessageEventTestHarness event={unresolvedMessage} userStore={lateAuthor} />
  </div>
</Story>

<Story name="Image attachment actions" asChild>
  <div class="max-w-xl p-4">
    <MessageEventTestHarness
      event={messageEvent({
        body: 'Long-press the image for attachment actions.',
        attachments: [
          {
            id: 'landscape',
            filename: 'landscape.svg',
            contentType: 'image/svg+xml',
            width: 600,
            height: 360,
            assetUrl: {
              url:
                'data:image/svg+xml,' +
                encodeURIComponent(
                  '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="360"><rect width="600" height="360" fill="#164e63"/><circle cx="300" cy="180" r="100" fill="#67e8f9"/></svg>'
                ),
              expiresAt: '2099-01-01T00:00:00Z'
            },
            thumbnailAssetUrl: null,
            videoProcessing: null
          }
        ]
      })}
    />
  </div>
</Story>
