<!--
@component
Shares cached identity reads between the popup and sidebar for the same
server session and profile. Stale data stays visible during a refresh.
-->
<script lang="ts">
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { createUserAPI } from '@chatto/client/api/users';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  import { providerIcon } from '$lib/auth/providerIcon';
  import VerifiedIdentitiesSection from './VerifiedIdentitiesSection.svelte';

  let { userId, sidebarKey }: { userId: string; sidebarKey?: string } = $props();
  const scope = useServerScope();

  const handles = createQuery(() => {
    const id = userId;
    return {
      queryKey: [...serverSessionQueryRoot(scope), 'user', id, 'atproto-handles'],
      queryFn: ({ signal }) => scope.connection.getAPI(createUserAPI).getAtprotoHandles(id, signal),
      retry: false
    };
  });
  const identities = $derived(
    (handles.data ?? []).map((identity) => ({
      identity,
      method: 'AT Protocol',
      icon: providerIcon('atproto')
    }))
  );
</script>

<VerifiedIdentitiesSection {identities} {sidebarKey} />
