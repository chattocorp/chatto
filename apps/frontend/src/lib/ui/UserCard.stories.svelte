<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent, waitFor } from 'storybook/test';
  import UserCard from './UserCard.svelte';
  import CompactActionButton from './CompactActionButton.svelte';

  const { Story } = defineMeta({ title: 'UI/UserCard', component: UserCard, tags: ['autodocs'] });
</script>

<script lang="ts">
  let lastAction = $state('No action yet');
  let speaking = $state(true);

  function speechLevel() {
    const time = performance.now() / 1000;
    return speaking && time % 7 < 4.5
      ? 0.005 + Math.pow((Math.sin(time * 8) + 1) / 2, 2) * 0.075
      : 0;
  }
</script>

<Story name="Bot identity" asChild>
  <div class="w-64">
    <UserCard name="Assistant" identity={{ isBot: true }} username="assistant_bot" {avatar} />
  </div>
</Story>

{#snippet avatar()}
  <span
    class="grid size-8 shrink-0 place-items-center rounded-full bg-surface-emphasized"
    aria-hidden="true">A</span
  >
{/snippet}

{#snippet badge()}
  <span class="shrink-0" title="Working remotely">🌴</span>
{/snippet}

<Story
  name="Member row"
  asChild
  play={async ({ canvas }) => {
    const menu = canvas.getByRole('button', { name: 'User options' });
    menu.focus();
    await waitFor(() => expect(getComputedStyle(menu.parentElement!).opacity).toBe('1'));
    await userEvent.click(canvas.getByText('Alice', { exact: true }));
    await expect(canvas.getByText('No action yet')).toBeVisible();
    await userEvent.click(canvas.getByRole('button', { name: 'User options' }));
    await expect(canvas.getByText('Profile opened')).toBeVisible();
  }}
>
  <div class="w-64">
    <UserCard
      name="Alice"
      username="alice"
      variant="row"
      {avatar}
      badges={badge}
      menu={{
        label: 'User options',
        onclick: () => (lastAction = 'Profile opened'),
        revealOnHover: true,
        oncontextmenu: () => (lastAction = 'Profile opened')
      }}
    />
    <p class="mt-2 text-muted" aria-live="polite">{lastAction}</p>
  </div>
</Story>

<Story name="Voice activity" asChild>
  <div class="flex w-72 flex-col gap-2">
    <UserCard
      name="Alice"
      username="alice"
      variant="card"
      {avatar}
      voiceLevel={speechLevel}
      menu={{ label: 'User options', onclick: () => {} }}
    />
    <UserCard
      name="Quiet participant"
      username="quiet"
      variant="card"
      {avatar}
      voiceLevel={() => 0}
    />
    <UserCard
      name="Camera participant"
      username="camera"
      variant="card"
      {avatar}
      voiceLevel={speechLevel}
    >
      <div class="grid aspect-video place-items-center bg-background text-muted">Video feed</div>
    </UserCard>
    <button class="btn-neutral btn" onclick={() => (speaking = !speaking)}
      >{speaking ? 'Stop speaking' : 'Start speaking'}</button
    >
  </div>
</Story>

<Story
  name="Equal heights in a constrained column"
  asChild
  play={async ({ canvas }) => {
    const cards = ['row', 'card', 'plain'].map((variant) => canvas.getByTestId(variant));
    const expected = 3 * parseFloat(getComputedStyle(document.documentElement).fontSize);
    for (const card of cards) await expect(card.getBoundingClientRect().height).toBe(expected);
  }}
>
  <div class="flex h-24 w-64 flex-col gap-2">
    <UserCard
      name="Alice"
      username="alice"
      {avatar}
      variant="row"
      testId="row"
      menu={{ label: 'User options', onclick: () => {} }}
    />
    <UserCard name="Alice" username="alice" {avatar} variant="card" testId="card" />
    <UserCard name="Alice" username="alice" {avatar} variant="plain" testId="plain" />
  </div>
</Story>

<Story name="Current user with separate avatar action" asChild>
  <div class="w-64">
    <UserCard name="Alice" username="alice" variant="card" badges={badge}>
      {#snippet avatar()}
        <button
          class="cursor-pointer rounded-full"
          aria-label="Change presence"
          onclick={() => (lastAction = 'Presence opened')}
        >
          <span class="grid size-8 place-items-center rounded-full bg-surface-emphasized">A</span>
        </button>
      {/snippet}
      {#snippet actions()}
        <CompactActionButton
          label="Privileged mode"
          onclick={() => (lastAction = 'Privileged mode opened')}
        >
          <span class="iconify icon-[uil--shield]"></span>
        </CompactActionButton>
      {/snippet}
    </UserCard>
    <p class="mt-2 text-muted" aria-live="polite">{lastAction}</p>
  </div>
</Story>

<Story
  name="Narrow participant with video"
  asChild
  play={async ({ canvas }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Mute' }));
    await expect(canvas.getByText('Muted', { exact: true })).toBeVisible();
    await userEvent.click(
      canvas.getByRole('button', { name: 'Alice with a very long display name @alice' })
    );
    await expect(canvas.getByText('Profile opened', { exact: true })).toBeVisible();
  }}
>
  <div class="w-56">
    <UserCard
      name="Alice with a very long display name"
      username="alice"
      variant="card"
      {avatar}
      identityAttributes={{ onclick: () => (lastAction = 'Profile opened') }}
    >
      {#snippet actions()}
        <CompactActionButton label="Mute" onclick={() => (lastAction = 'Muted')}>
          <span class="iconify icon-[uil--volume-up]"></span>
        </CompactActionButton>
        <CompactActionButton
          label="Participant options"
          onclick={() => (lastAction = 'Options opened')}
        >
          <span class="iconify icon-[uil--ellipsis-v]"></span>
        </CompactActionButton>
      {/snippet}
      <div class="grid aspect-video place-items-center bg-background text-muted">Video feed</div>
    </UserCard>
  </div>
  <p class="mt-2 text-muted" aria-live="polite">{lastAction}</p>
</Story>

<Story name="Voice glow levels" asChild>
  <div class="flex w-72 flex-col gap-2">
    <UserCard
      name="Quiet speech"
      username="quiet"
      variant="card"
      {avatar}
      voiceLevel={() => 0.005}
    />
    <UserCard
      name="Normal speech"
      username="normal"
      variant="card"
      {avatar}
      voiceLevel={() => 0.03}
    />
    <UserCard name="Loud speech" username="loud" variant="card" {avatar} voiceLevel={() => 0.2} />
  </div>
</Story>

<Story name="Right to left" asChild>
  <div class="w-64" dir="rtl">
    <UserCard
      name="ليلى"
      username="layla"
      variant="row"
      {avatar}
      badges={badge}
      identityAttributes={{ onclick: () => (lastAction = 'Profile opened') }}
    />
  </div>
</Story>
