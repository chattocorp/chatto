<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import MessageView from './MessageView.svelte';

  const { Story } = defineMeta({
    title: 'Components/Messages/MessageView',
    component: MessageView,
    tags: ['autodocs'],
    args: {
      eventId: 'incoming',
      actor: null,
      displayName: 'Unknown user',
      body: 'The message appears immediately.'
    }
  });
  const actor = {
    id: 'author',
    login: 'author',
    displayName: 'Message author',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE
  };
</script>

<script lang="ts">
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  createPresenceCache();
  createUserProfileCache();
</script>

<Story name="Loading author" args={{ authorLoading: true, missingActorIsDeleted: false }} />
<Story name="Unknown author" args={{ missingActorIsDeleted: false }} />
<Story name="Deleted author" />
<Story name="Loaded author" args={{ actor, displayName: actor.displayName }} />
<Story
  name="Loaded bot"
  args={{ actor: { ...actor, isBot: true }, displayName: actor.displayName }}
/>
