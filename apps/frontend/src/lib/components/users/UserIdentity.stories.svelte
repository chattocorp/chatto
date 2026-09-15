<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import UserIdentity from './UserIdentity.svelte';

  const { Story } = defineMeta({
    title: 'Components/User identity',
    component: UserIdentity,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';

  provideServerScope({
    serverId: 'storybook',
    connection: {} as ServerConnection,
    store: {
      permissions: { loaded: true, canStartDMs: true, canAdminViewUsers: false }
    } as ServerStateStore,
    isCurrent: () => true
  });
  const user = { id: 'owner', login: 'alice', displayName: 'Alice', avatarUrl: null };
  const onSendMessage = fn();
  const onOpenProfile = fn();
</script>

<Story name="Compact owner with room actions" asChild>
  <div class="flex items-center gap-2 p-4">
    <span class="text-muted">Owned by</span>
    <UserIdentity {user} size="xs" openOnClick {onSendMessage} {onOpenProfile} />
  </div>
</Story>

<Story name="Long name" asChild>
  <div class="flex w-64 items-center gap-2 p-4">
    <span class="shrink-0 text-muted">Owned by</span>
    <UserIdentity
      user={{ ...user, displayName: 'Alice with a very long display name' }}
      size="xs"
      openOnClick
      {onSendMessage}
      {onOpenProfile}
    />
  </div>
</Story>
