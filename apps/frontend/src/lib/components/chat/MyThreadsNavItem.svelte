<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import * as m from '$lib/i18n/messages';

  let {
    active,
    hasUnread = false,
    hasNotification = false
  }: { active: boolean; hasUnread?: boolean; hasNotification?: boolean } = $props();
</script>

<a
  href={resolve('/chat/[serverId]/threads', { serverId: serverIdToSegment(getActiveServer()) })}
  class={['sidebar-item', active ? 'bg-surface' : '']}
>
  <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
  {m['chat.threads.title']()}
  {#if hasNotification || hasUnread}
    <UnreadDot
      class="ml-auto"
      color={hasNotification ? 'warning' : 'neutral'}
      testid="my-threads-unread-dot"
    />
  {/if}
</a>
