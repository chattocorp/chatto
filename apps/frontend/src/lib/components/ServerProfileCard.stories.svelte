<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import ServerProfileCard, { type ServerProfileCardProfile } from './ServerProfileCard.svelte';

  const { Story } = defineMeta({
    title: 'Components/ServerProfileCard',
    component: ServerProfileCard,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared server cards for the Server Directory and Neighbors admin. The Inset Frame surrounds a recessed banner and optional status footer; identity, description, and actions sit on the raised surface.'
        }
      }
    }
  });

  const profile: ServerProfileCardProfile = {
    name: 'The Extremely Long Neighbourhood Server Name',
    description: 'A calm place for thoughtful conversations and small communities.',
    iconUrl: null,
    bannerUrl: null
  };
</script>

<Story name="Populated directory" parameters={{ layout: 'fullscreen' }} asChild>
  <div class="mx-auto flex max-w-7xl flex-col gap-6 p-6">
    <h1 class="text-2xl font-bold text-text-top">Server Directory</h1>
    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      {#each [{ name: 'The Greenhouse', host: 'greenhouse', description: 'A growing community of gardeners, plant collectors, and people who keep trying to keep their basil alive.' }, { name: 'Night Shift', host: 'nightshift', description: 'Late-night conversations, shared playlists, and good company across every time zone.' }, { name: 'Pixel Workshop', host: 'pixels', description: 'Small games, big ideas. Share your work, trade feedback, and build something playful.' }, { name: 'Common Ground', host: 'common', description: 'Your neighbourhood, online. Local projects, weekend plans, and friendly introductions.' }, { name: 'Orbit', host: 'orbit', description: 'For curious minds looking up. Space news, telescope tips, and the occasional rocket launch.' }, { name: 'The Reading Room', host: 'reading', description: 'One more chapter. Book clubs, unexpected recommendations, and conversations that go beyond the last page.' }, { name: 'Trail Mix', host: 'trails', description: 'Find your next path. Hiking routes, muddy boots, and stories from the great outdoors.' }, { name: 'Open Kitchen', host: 'kitchen', description: 'Recipes worth repeating, ambitious experiments, and a place at the table for everyone.' }] as server, index (server.host)}
        {#snippet footer()}{index < 3 ? 'Joined' : 'Recommended by Common Ground'}{/snippet}
        <ServerProfileCard
          origin={`https://${server.host}.example`}
          profile={{ ...server, iconUrl: null, bannerUrl: null }}
          {footer}
          onactivate={() => {}}
          actionLabel={index < 3 ? 'Open' : 'Join'}
          headingTag="h2"
        />
      {/each}
    </div>
  </div>
</Story>

<Story name="Admin profile" asChild>
  <div class="w-full max-w-sm p-4">
    <ServerProfileCard origin="https://long-neighbourhood.example" {profile} />
  </div>
</Story>

<Story name="Equal-height descriptions" asChild>
  <div class="grid grid-cols-1 gap-4 p-4 sm:grid-cols-3">
    {#each ['', 'A short introduction.', 'A community for long conversations.\n\nThis description has several paragraphs so you can scroll through the complete introduction without changing the height of the card.\n\nEveryone is welcome.\n\nBring your questions and ideas.\n\nThere is always more to talk about.'] as description, index (description)}
      {#snippet footer()}Added by you{/snippet}
      <ServerProfileCard
        origin={`https://community-${index + 1}.example`}
        profile={{ ...profile, description }}
        {footer}
        onactivate={() => {}}
        actionLabel="Join"
      />
    {/each}
  </div>
</Story>

<Story name="Saved server" asChild>
  <div class="w-full max-w-sm p-4">
    {#snippet footer()}Joined{/snippet}
    <ServerProfileCard
      origin="https://community.example"
      {profile}
      href="https://community.example"
      actionLabel="Open"
      {footer}
    />
  </div>
</Story>

<Story name="Mobile directory" parameters={{ layout: 'fullscreen' }} asChild>
  <div class="grid w-full grid-cols-2 gap-2 p-4">
    {#each ['Chatto HQ', 'The Extremely Long Neighbourhood Server Name'] as name (name)}
      {#snippet footer()}Joined{/snippet}
      <ServerProfileCard
        origin="https://long-neighbourhood.example"
        profile={{ ...profile, name }}
        href="https://long-neighbourhood.example"
        actionLabel="Open"
        {footer}
      />
    {/each}
  </div>
</Story>

<Story name="Admin actions" asChild>
  <div class="mx-auto w-full max-w-md p-4 text-start">
    {#snippet actions()}
      <Button variant="secondary">Edit</Button>
    {/snippet}
    <ServerProfileCard origin="https://community.example" {profile} {actions} headingTag="h2" />
  </div>
</Story>

<Story name="External handoff" asChild>
  <div class="w-full max-w-sm p-4">
    {#snippet footer()}Recommended by Chatto HQ{/snippet}
    <ServerProfileCard
      origin="https://old-neighbourhood.example"
      {profile}
      href="https://old-neighbourhood.example"
      opensInNewTab
      actionLabel="Open in new tab"
      {footer}
    />
  </div>
</Story>
