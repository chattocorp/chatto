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
  const zalgoBody = `Congs\n\nZ${'\u0301'.repeat(40)}`;
</script>

<script lang="ts">
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  provideUserProfiles();
  let opened = $state(false);
</script>

<Story name="Unknown author" args={{ missingActorIsDeleted: false }} />
<Story name="Deleted author" />
<Story name="Interactive body" asChild>
  <MessageView
    eventId="interactive"
    actor={null}
    displayName="Message author"
    body="Click this text to open the thread, or select a word. [This link](https://example.com) keeps its own action."
    onBodyActivate={() => (opened = true)}
  />
  {#if opened}<p role="status">Thread opened</p>{/if}
</Story>
<Story name="Loaded author" args={{ actor, displayName: actor.displayName }} />
<Story
  name="Descender author"
  args={{ actor: { ...actor, displayName: 'gg' }, displayName: 'gg', body: 'Congs' }}
/>
<Story
  name="Descender author with combining marks"
  args={{ actor: { ...actor, displayName: 'gg' }, displayName: 'gg', body: zalgoBody }}
/>
<Story
  name="Loaded bot"
  args={{ actor: { ...actor, isBot: true }, displayName: actor.displayName }}
/>
