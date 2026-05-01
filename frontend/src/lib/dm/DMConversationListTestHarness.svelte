<!--
@component

Test-only wrapper around `DMConversationList`. Constructs a real
`DMConversationsStore`, sets the context, and seeds the conversations array
so component-level tests can exercise the rendered view without going through
the layout's full subscription wiring.

Lives next to the component (rather than under test-utils/) because it's
specific to this component's context shape.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import {
    DMConversationsStore,
    setDMConversationsStore,
    type DMConversation
  } from '$lib/state/dm/conversations.svelte';
  import DMConversationList from './DMConversationList.svelte';

  let {
    initialConversations,
    activeConversationId
  }: {
    initialConversations: DMConversation[];
    activeConversationId?: string;
  } = $props();

  const store = new DMConversationsStore();
  store.conversations = untrack(() => initialConversations);
  setDMConversationsStore(store);
</script>

<DMConversationList {activeConversationId} />
