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
