<!-- @component Render the resolved conversation; SvelteKit owns requests and stale navigation. -->
<script lang="ts">
  import { invalidate } from '$app/navigation';
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { m } from '$lib/i18n/messages';
  import Room from '../../[roomId]/Room.svelte';
  import { EmptyState, LoadingPage, PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import type { LayoutData } from './$types';

  let { data, children }: { data: LayoutData; children?: Snippet } = $props();
  // Child route data changes must not restart this await block or remount the composer.
  const conversation = $derived(data.conversationRoomId);
</script>

<PageTitle />

{#await conversation}
  <LoadingPage />
{:then roomId}
  {#if roomId && page.route.id !== '/chat/[serverId]/user/[userId]/m/[messageId]'}
    <Room {roomId} threadId={page.params.threadId} routeMessageId={page.params.messageId} />
  {:else if !roomId}
    <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.generic')}>
      <Button variant="secondary" onclick={() => invalidate('chatto:conversation')}
        >{m('common.retry')}</Button
      >
    </EmptyState>
  {:else}
    <LoadingPage />
  {/if}
{/await}

{@render children?.()}
