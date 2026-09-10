<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import UserList from './UserList.svelte';

  const { Story } = defineMeta({
    title: 'Admin/UserList',
    component: UserList,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  provideServerScope({
    serverId: 'storybook',
    connection: {} as ServerConnection,
    store: {} as ServerStateStore,
    isCurrent: () => true
  });
  const users = [
    { id: 'user-1', login: 'alex', displayName: 'Alex' },
    { id: 'user-2', login: 'sam', displayName: 'Sam' }
  ];
</script>

<Story name="Partial page" asChild>
  <Panel title="Role members" noPadding>
    <UserList {users} totalCount={45} hasMore loadingMore clickable={false} />
  </Panel>
</Story>

<Story name="Complete list" asChild>
  <Panel title="Role members" noPadding>
    <UserList {users} clickable={false} />
  </Panel>
</Story>
