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

  let { userId }: { userId: string } = $props();
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
  <div
    class="my-2 flex min-w-0 items-start gap-2.5 rounded-lg border border-border bg-surface/60 px-2.5 py-2 text-start"
  >
    <span
      class="mt-0.5 grid size-7 shrink-0 place-items-center rounded-md bg-surface-emphasized text-muted"
    >
      <span aria-hidden="true" class="iconify icon-[mdi--at]"></span>
    </span>
    <div class="min-w-0">
      <div class="text-xs font-medium text-muted">AT Protocol</div>
      {#each handles.data as handle (handle)}
        <div class="text-sm font-medium wrap-anywhere"><bdi dir="ltr">@{handle}</bdi></div>
      {/each}
    </div>
  </div>
{/if}
