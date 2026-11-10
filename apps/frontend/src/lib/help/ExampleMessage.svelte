<!-- @component A local message example using the shared identity and Markdown components. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { MarkdownHtml, UserCard, LoadingFog } from '$lib/ui';
  import { renderMarkdown } from '$lib/markdown';
  let {
    name = 'Alex',
    body,
    children
  }: { name?: string; body: string; children?: Snippet } = $props();
  const user = $derived({
    id: `help-${name}`,
    login: name.toLowerCase(),
    displayName: name,
    avatarUrl: null,
    deleted: false,
    presenceStatus: 0,
    customStatus: null
  });
  const rendered = $derived(renderMarkdown(body));
</script>

<div class="flex min-w-0 flex-col gap-2">
  <UserCard {name}>
    {#snippet avatar()}<UserAvatar {user} useLiveProfile={false} size="sm" />{/snippet}
  </UserCard>
  <!-- MarkdownHtml uses display:contents. Give inline text a block parent, never a flex parent. -->
  <div class="prose ms-11 min-w-0 leading-relaxed break-words">
    {#await rendered}<LoadingFog class="h-8" />{:then html}<MarkdownHtml {html} />{/await}
  </div>
  {#if children}<div class="ms-11">{@render children()}</div>{/if}
</div>
