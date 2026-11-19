<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, within } from 'storybook/test';
  import HelpArtwork from '$lib/help/HelpArtwork.svelte';
  import DestinationCard from './DestinationCard.svelte';
  const { Story } = defineMeta({ title: 'Settings/Destination card', component: DestinationCard });
</script>

<Story
  name="Help and settings"
  asChild
  play={async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const help = canvas.getByRole('link', { name: 'Using Chatto' });
    await expect(help).toHaveAccessibleDescription(
      'Threads, notifications, and connection problems.'
    );
    await expect(canvas.getByRole('link', { name: 'Appearance' })).toHaveAccessibleDescription(
      'Choose the colours and theme for this device.'
    );
  }}
>
  <div class="grid max-w-2xl grid-cols-1 gap-4 sm:grid-cols-2">
    <DestinationCard
      href="/help/conversations"
      label="Using Chatto"
      description="Threads, notifications, and connection problems."
      icon="iconify icon-[uil--comments]"
    >
      {#snippet artwork()}<HelpArtwork topic="conversations" />{/snippet}
    </DestinationCard>
    <DestinationCard
      href="/chat/preferences/appearance"
      label="Appearance"
      description="Choose the colours and theme for this device."
      icon="iconify icon-[uil--palette]"
    />
  </div>
</Story>
