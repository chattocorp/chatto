<script lang="ts">
  import { resolve } from '$app/paths';
  import { useActiveServerScope } from '$lib/state/server/activeServerScope.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import * as m from '$lib/i18n/messages';

  let { active }: { active: boolean } = $props();

  const server = useActiveServerScope();
  const notificationStore = $derived(server.notifications);

  const hasUnread = $derived(
    notificationStore.notifications.some((n) => notificationTarget(n).threadRootId !== null)
  );
</script>

<a
  href={resolve('/chat/[serverId]/threads', { serverId: server.segment })}
  class={['sidebar-item', active ? 'bg-surface-100' : '']}
>
  <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
  {m['chat.threads.title']()}
  {#if hasUnread}
    <UnreadDot class="ml-auto" testid="my-threads-unread-dot" />
  {/if}
</a>
