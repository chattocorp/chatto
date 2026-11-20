<!--
@component
Shares cached identity reads between the popup and sidebar for the same
server session and profile. Stale data stays visible during a refresh.
-->
<script lang="ts">
  import { createQuery } from '$lib/query/client';
  import { profileVerifiedIdentitiesKey } from '$lib/query/keys';
  import { createUserAPI } from '@chatto/client/api/users';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  import VerifiedIdentitiesSection from './VerifiedIdentitiesSection.svelte';

  let { userId, sidebarKey }: { userId: string; sidebarKey?: string } = $props();
  const scope = useServerScope();

  const identities = createQuery(() => {
    const id = userId;
    return {
      queryKey: profileVerifiedIdentitiesKey(scope, id),
      queryFn: ({ signal }) =>
        scope.connection.getAPI(createUserAPI).getVerifiedIdentities(id, signal),
      retry: false
    };
  });
</script>

<VerifiedIdentitiesSection identities={identities.data ?? []} {sidebarKey} />
