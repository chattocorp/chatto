<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { expect } from 'storybook/test';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { providerIcon } from '$lib/auth/providerIcon';
  import VerifiedIdentitiesSection from '$lib/components/users/VerifiedIdentitiesSection.svelte';
  import UserCard from './UserCard.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import SelfVerificationPill from './SelfVerificationPill.svelte';

  const { Story } = defineMeta({
    title: 'UI/SelfVerificationPill',
    parameters: {
      docs: {
        description: {
          component:
            'Use one pill per self-verification identity. The provider icon and identity share a green tint with a transparent 1px gap. The icon stays visible when the identity is shortened. Hover to read the full identity.'
        }
      }
    },
    component: SelfVerificationPill,
    tags: ['autodocs'],
    args: { method: 'AT Protocol', icon: providerIcon('atproto'), identity: 'hmans.dev' }
  });
</script>

<Story name="AT Protocol" />
<Story
  name="Long handle"
  asChild
  play={async ({ canvas }) => {
    const identity = canvas.getByText('long-handle.research.community.example');
    const pill = canvas.getByTitle('AT Protocol · long-handle.research.community.example');
    await expect(identity).toBeVisible();
    await expect(getComputedStyle(identity).whiteSpace).toBe('nowrap');
    await expect(getComputedStyle(identity).textOverflow).toBe('ellipsis');
    await expect(identity.scrollWidth).toBeGreaterThan(identity.clientWidth);
    await expect(identity.clientHeight).toBeCloseTo(
      parseFloat(getComputedStyle(identity).lineHeight),
      0
    );
    await expect(identity.getBoundingClientRect().right).toBeLessThanOrEqual(
      pill.getBoundingClientRect().right
    );
    await expect(pill.getBoundingClientRect().right).toBeLessThanOrEqual(
      pill.parentElement!.getBoundingClientRect().right
    );
  }}
>
  <div class="w-40">
    <SelfVerificationPill
      method="AT Protocol"
      icon={providerIcon('atproto')}
      identity="long-handle.research.community.example"
    />
  </div>
</Story>

<Story name="Profile sidebar" asChild>
  <div class="w-64 max-w-full p-4">
    <UserCard variant="card" name="Alice Example" username="alice">
      {#snippet avatar()}
        <UserAvatar
          user={{
            id: 'alice',
            login: 'alice',
            displayName: 'Alice Example',
            presenceStatus: PresenceStatus.OFFLINE
          }}
          size="sm"
          useLiveProfile={false}
        />
      {/snippet}
    </UserCard>
    <VerifiedIdentitiesSection
      sidebarKey="story-profile-identities"
      identities={[{ method: 'AT Protocol', icon: providerIcon('atproto'), identity: 'hmans.dev' }]}
    />
  </div>
</Story>
