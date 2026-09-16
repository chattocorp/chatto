<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, fireEvent, userEvent, within } from 'storybook/test';
  import ParticipantCardMenu from './ParticipantCardMenu.svelte';

  const { Story } = defineMeta({
    title: 'Voice/Participant audio',
    component: ParticipantCardMenu,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import type { ParticipantAudioPreferences } from '$lib/state/server/callPreferences.svelte';

  let settings = $state<ParticipantAudioPreferences>({ voiceVolume: 150, streamVolume: 60 });
</script>

<Story
  name="Controls"
  asChild
  play={async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Participant audio' }));
    const menu = within(document.body);
    const voice = menu.getByRole('slider', { name: /Voice volume/ });
    await expect(voice).toHaveAttribute('max', '200');
    await expect(menu.queryByRole('button', { name: 'Mute locally' })).not.toBeInTheDocument();
    await expect(voice).toHaveValue('150');
    await expect(voice).toHaveAttribute('step', '5');
    fireEvent.input(voice, { target: { value: '100' } });
    await expect(voice).toHaveValue('100');
    await expect(menu.queryByRole('button', { name: 'Reset volume' })).not.toBeInTheDocument();
  }}
>
  <div class="w-72 rounded-lg border border-border bg-surface p-2">
    <ParticipantCardMenu
      {settings}
      onVolumeChange={(control, value) => (settings = { ...settings, [control]: value })}
    />
  </div>
</Story>

<Story name="Boost unavailable" asChild>
  <div class="w-72 rounded-lg border border-border bg-surface p-2">
    <ParticipantCardMenu
      {settings}
      boostAvailable={false}
      onVolumeChange={(control, value) => (settings = { ...settings, [control]: value })}
    />
  </div>
</Story>
