<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import ServerProfileCard, { type ServerProfileCardProfile } from './ServerProfileCard.svelte';

  const { Story } = defineMeta({
    title: 'Components/ServerProfileCard',
    component: ServerProfileCard,
    tags: ['autodocs']
  });

  const profile: ServerProfileCardProfile = {
    name: 'The Extremely Long Neighbourhood Server Name',
    description: 'A calm place for thoughtful conversations and small communities.',
    iconUrl: null,
    bannerUrl: null
  };
</script>

<Story name="Joined" asChild>
  <div class="w-full max-w-sm p-4">
    <ServerProfileCard origin="https://long-neighbourhood.example" {profile} badge="Joined" />
  </div>
</Story>

<Story name="Equal-height descriptions" asChild>
  <div class="grid grid-cols-1 gap-4 p-4 sm:grid-cols-3">
    {#each ['', 'A short introduction.', 'A community for long conversations.\n\nThis description has several paragraphs so you can scroll through the complete introduction without changing the height of the card.\n\nEveryone is welcome.\n\nBring your questions and ideas.\n\nThere is always more to talk about.'] as description (description)}
      {#snippet actions()}<Button fullWidth>Join</Button>{/snippet}
      <ServerProfileCard
        origin="https://community.example"
        profile={{ ...profile, description }}
        {actions}
      />
    {/each}
  </div>
</Story>

<Story name="Saved server" asChild>
  <div class="w-full max-w-sm p-4">
    {#snippet actions()}
      <Button variant="secondary" fullWidth>Open</Button>
    {/snippet}
    <ServerProfileCard origin="https://community.example" {profile} {actions} />
  </div>
</Story>

<Story name="First-run preview" asChild>
  <div class="mx-auto w-full max-w-md p-4 text-start">
    {#snippet actions()}
      <Button fullWidth>Join</Button>
    {/snippet}
    <ServerProfileCard origin="https://community.example" {profile} {actions} headingTag="h2" />
  </div>
</Story>

<Story name="External handoff" asChild>
  <div class="w-full max-w-sm p-4">
    {#snippet actions()}
      <Button href="https://old-neighbourhood.example" opensInNewTab variant="secondary" fullWidth>
        <span>Open in new tab</span>
        <span class="iconify icon-[uil--external-link-alt]" aria-hidden="true"></span>
      </Button>
    {/snippet}
    <ServerProfileCard
      origin="https://old-neighbourhood.example"
      {profile}
      iconHref="https://old-neighbourhood.example"
      iconOpensInNewTab
      iconActionLabel="Open in new tab"
      {actions}
    />
  </div>
</Story>
