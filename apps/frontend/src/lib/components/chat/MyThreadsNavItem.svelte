<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import * as m from '$lib/i18n/messages';

  let { active, hasUnread }: { active: boolean; hasUnread: boolean } = $props();
</script>

<a
  href={resolve('/chat/[serverId]/threads', { serverId: serverIdToSegment(getActiveServer()) })}
  class={['sidebar-item', active ? 'bg-surface' : '']}
>
  <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
  {m['chat.threads.title']()}
  {#if hasUnread}
    <UnreadDot class="ml-auto" testid="my-threads-unread-dot" />
  {/if}
</a>
