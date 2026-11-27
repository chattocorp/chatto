<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, fn, within } from 'storybook/test';
  import HelpArtwork from '$lib/help/HelpArtwork.svelte';
  import DestinationCard from './DestinationCard.svelte';
  const general = {
    href: '/chat/example/manage/server/general',
    label: 'General',
    description: 'Manage server settings.',
    icon: 'iconify icon-[uil--setting]'
  };
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

<Story
  name="Muted shield"
  args={{ ...general, requestAccess: fn() }}
  play={async ({ canvasElement, args, userEvent }) => {
    const button = within(canvasElement).getByRole('button', { name: general.label });
    await expect(button).toHaveAttribute('aria-haspopup', 'dialog');
    await expect(button).toHaveAccessibleDescription(
      'Enable privileged mode Manage server settings.'
    );
    await userEvent.click(button);
    await expect(args.requestAccess).toHaveBeenCalledOnce();
  }}
/>

<Story
  name="Active shield"
  args={{ ...general, shieldActive: true }}
  play={async ({ canvasElement }) => {
    const link = within(canvasElement).getByRole('link', { name: general.label });
    await expect(link).toHaveAttribute('href', general.href);
    await expect(link).toHaveAccessibleDescription('Shield Mode active Manage server settings.');
  }}
/>
