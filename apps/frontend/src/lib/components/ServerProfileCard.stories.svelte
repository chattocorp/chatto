<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { PublicServerInfo } from '$lib/api-client/server';
  import { Button } from '$lib/ui/form';
  import ServerProfileCard from './ServerProfileCard.svelte';

  const { Story } = defineMeta({
    title: 'Components/ServerProfileCard',
    component: ServerProfileCard,
    tags: ['autodocs']
  });

  const profile: PublicServerInfo = {
    name: 'The Extremely Long Neighbourhood Server Name',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'open',
    welcomeMessage: null,
    description: 'A calm place for thoughtful conversations and small communities.',
    iconUrl: null,
    bannerUrl: null,
    authProviders: []
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
      profile={{ ...profile, version: '0.4.19' }}
      iconHref="https://old-neighbourhood.example"
      iconOpensInNewTab
      iconActionLabel="Open in new tab"
      {actions}
    />
  </div>
</Story>
