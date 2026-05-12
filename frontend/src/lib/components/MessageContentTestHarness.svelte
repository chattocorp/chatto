<!--
@component

Test-only wrapper around `MessageContent`. Provides the active-server
context that `MessageContent` reads via `getActiveServer()`. The returned
ID is empty by default — `tryGetStore('')` returns undefined and
`MessageContent`'s `viewerLogin` falls through to undefined, which
`wrapValidMentions` treats as "no self-mention." Sufficient for tests
that exercise rendering / mention wiring without a registered server.
-->
<script lang="ts">
  import { setActiveServer } from '$lib/state/activeServer.svelte';
  import type { RoomMember } from '$lib/mentions';
  import MessageContent from './MessageContent.svelte';

  let {
    body,
    members = [],
    edited = false,
    activeServerId = ''
  }: {
    body: string;
    members?: RoomMember[];
    edited?: boolean;
    activeServerId?: string;
  } = $props();

  // svelte-ignore state_referenced_locally
  setActiveServer(() => activeServerId);
</script>

<MessageContent {body} {members} {edited} />
