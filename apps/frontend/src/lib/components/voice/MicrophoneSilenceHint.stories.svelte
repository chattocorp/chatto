<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MicrophoneSilenceHint from './MicrophoneSilenceHint.svelte';
  const { Story } = defineMeta({
    title: 'Voice/Microphone silence hint',
    component: MicrophoneSilenceHint,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import UserCard from '$lib/ui/UserCard.svelte';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  const call = $state({
    connected: true,
    microphoneSilent: true,
    audioDevices: [],
    audioOutputDevices: [],
    videoDevices: [],
    refreshDevices: async () => {}
  });
  provideServerScope({
    serverId: 'microphone-hint-story',
    store: { voiceCall: call } as unknown as ServerStateStore,
    get connection(): never {
      throw new Error('This fixture does not use a server connection');
    },
    isCurrent: () => true
  });
</script>

<Story name="Check microphone" asChild>
  <div class="flex w-72 flex-col gap-3">
    <UserCard name="Alice" username="alice" variant="card">
      {#snippet avatar()}
        <span class="bg-elevated grid size-8 place-items-center rounded-full">A</span>
      {/snippet}
      {#snippet actions()}<MicrophoneSilenceHint />{/snippet}
    </UserCard>
    <button
      class="btn-neutral btn"
      onclick={() => (call.microphoneSilent = !call.microphoneSilent)}
    >
      {call.microphoneSilent ? 'Simulate audio recovery' : 'Simulate silent input'}
    </button>
  </div>
</Story>
