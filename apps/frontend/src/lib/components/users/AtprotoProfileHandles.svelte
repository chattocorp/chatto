<!--
@component
Shows linked internet handles on the public profile. Each read belongs to
the selected server and member and stops when the profile closes or changes.
-->
<script lang="ts">
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { createUserAPI } from '@chatto/client/api/users';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  import { providerIcon } from '$lib/auth/providerIcon';
  import SelfVerificationPill from './SelfVerificationPill.svelte';

  let { userId, class: className }: { userId: string; class?: string } = $props();
  const scope = useServerScope();

  const handles = createQuery(() => {
    const id = userId;
    return {
      queryKey: [...serverSessionQueryRoot(scope), 'user', id, 'atproto-handles'],
      queryFn: ({ signal }) => scope.connection.getAPI(createUserAPI).getAtprotoHandles(id, signal),
      gcTime: 0,
      retry: false
    };
  });
</script>

{#if handles.data?.length}
  <div class={['flex min-w-0 flex-wrap gap-1', className]}>
    {#each handles.data as handle (handle)}
      <SelfVerificationPill method="AT Protocol" icon={providerIcon('atproto')} identity={handle} />
    {/each}
  </div>
{/if}
