<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { PublicServerInfo } from '$lib/api-client/server';
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

<Story name="With testimonials" asChild>
  <div class="w-full max-w-sm p-4">
    {#snippet details()}
      <section class="flex flex-col gap-2" aria-label="Testimonials for the server">
        <figure class="surface-box p-3">
          <blockquote class="text-sm leading-relaxed text-text">
            A welcoming community with careful moderation and excellent conversations.
          </blockquote>
          <figcaption class="mt-2 text-sm font-medium text-muted">
            Recommended by Garden Chat
          </figcaption>
        </figure>
        <figure class="surface-box p-3">
          <blockquote class="text-sm leading-relaxed text-text">
            Their weekly events make it easy to meet people.
          </blockquote>
          <figcaption class="mt-2 text-sm font-medium text-muted">
            Recommended by Tiny Town
          </figcaption>
        </figure>
      </section>
    {/snippet}
    <ServerProfileCard
      origin="https://long-neighbourhood.example"
      {profile}
      badge="Joined"
      {details}
    />
  </div>
</Story>
