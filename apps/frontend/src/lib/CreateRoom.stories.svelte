<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CreateRoom from './CreateRoom.svelte';

  const { Story } = defineMeta({
    title: 'Components/CreateRoom',
    component: CreateRoom,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import type { RoomCommandAPI } from '$lib/api-client/rooms';
  import { RoomThreadingMode } from '$lib/roomThreading';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  import { Button } from '$lib/ui/form';

  let visible = $state(false);

  const roomAPI: Pick<RoomCommandAPI, 'createRoom' | 'joinRoom'> = {
    createRoom: async (input) => ({
      id: 'design-systems',
      name: input.name,
      description: input.description ?? '',
      archived: false,
      groupId: input.groupId,
      universal: input.universal ?? false,
      slowModeSeconds: 0,
      threadingMode: input.threadingMode ?? RoomThreadingMode.ENABLED
    }),
    joinRoom: async () => null
  };

  provideServerScope({
    serverId: 'storybook',
    connection: {
      getAPI: () => roomAPI
    } as unknown as ServerConnection,
    store: {} as ServerStateStore,
    isCurrent: () => true
  });
</script>

<Story name="Default" asChild>
  <Button onclick={() => (visible = true)}>Create room...</Button>
  <CreateRoom
    bind:visible
    groupId="product"
    onclose={() => (visible = false)}
    onroomcreated={() => (visible = false)}
  />
</Story>
