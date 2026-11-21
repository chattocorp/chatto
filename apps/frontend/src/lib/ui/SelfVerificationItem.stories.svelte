<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { expect } from 'storybook/test';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { providerIcon } from '$lib/auth/providerIcon';
  import VerifiedIdentitiesSection from '$lib/components/users/VerifiedIdentitiesSection.svelte';
  import UserCard from './UserCard.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import SelfVerificationItem from './SelfVerificationItem.svelte';

  const { Story } = defineMeta({
    title: 'UI/SelfVerificationItem',
    parameters: {
      docs: {
        description: {
          component:
            'Use one item per self-verification identity: the provider icon, the identity in normal text, and a green check. The icons stay visible when the identity is shortened. Hover to read the full identity.'
        }
      }
    },
    component: SelfVerificationItem,
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
    const item = canvas.getByTitle('AT Protocol · long-handle.research.community.example');
    await expect(identity).toBeVisible();
    await expect(getComputedStyle(identity).whiteSpace).toBe('nowrap');
    await expect(getComputedStyle(identity).textOverflow).toBe('ellipsis');
    await expect(identity.scrollWidth).toBeGreaterThan(identity.clientWidth);
    await expect(identity.clientHeight).toBeCloseTo(
      parseFloat(getComputedStyle(identity).lineHeight),
      0
    );
    await expect(identity.getBoundingClientRect().right).toBeLessThanOrEqual(
      item.getBoundingClientRect().right
    );
    await expect(item.getBoundingClientRect().right).toBeLessThanOrEqual(
      item.parentElement!.getBoundingClientRect().right
    );
  }}
>
  <div class="w-40">
    <SelfVerificationItem
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
      identities={[{ method: 'atproto', methodLabel: 'AT Protocol', value: 'hmans.dev' }]}
    />
  </div>
</Story>

<Story name="Profile popup" asChild>
  <div class="w-64 menu">
    <div class="flex flex-col gap-1">
      <div class="flex items-center gap-3 menu-section p-3 text-start">
        <UserAvatar
          user={{
            id: 'alice',
            login: 'alice',
            displayName: 'Alice Example',
            presenceStatus: PresenceStatus.OFFLINE
          }}
          size="md"
          useLiveProfile={false}
        />
        <div class="min-w-0 flex-1">
          <div class="truncate font-semibold">Alice Example</div>
          <div class="truncate text-xs text-muted">@alice</div>
        </div>
      </div>
      <VerifiedIdentitiesSection
        identities={[
          { method: 'atproto', methodLabel: 'AT Protocol', value: 'hmans.dev' },
          {
            method: 'atproto',
            methodLabel: 'AT Protocol',
            value: 'long-handle.research.community.example'
          }
        ]}
      />
      <div class="menu-section px-3 py-2 text-start text-sm">Builds chat things.</div>
    </div>
  </div>
</Story>
