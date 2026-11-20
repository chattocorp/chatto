<script module lang="ts">
  import { expect } from 'storybook/test';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { providerIcon } from '$lib/auth/providerIcon';
  import SelfVerificationPill from './SelfVerificationPill.svelte';

  const { Story } = defineMeta({
    title: 'Components/Users/SelfVerificationPill',
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
    const pill = canvas.getByTitle('AT Protocol');
    await expect(identity).toBeVisible();
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
