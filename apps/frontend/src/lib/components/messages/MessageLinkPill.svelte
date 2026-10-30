<!-- @component Compact message identity with an author when known and a room or server fallback. -->
<script lang="ts">
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { handleRenderedLinkClick, renderedLinkTarget } from '$lib/navigation/renderedLink';
  import UserAvatar from '../UserAvatar.svelte';
  import { m } from '$lib/i18n/messages';

  let {
    href,
    name,
    context,
    user
  }: { href: string; name: string; context: string; user?: UserAvatarUserView } = $props();
  const destination = $derived(renderedLinkTarget(href));
</script>

<a
  {...destination}
  class="entity-link max-w-[min(100%,36ch)]"
  class:entity-link-with-image={!!user}
  data-entity-kind={user ? 'user' : 'message'}
  data-message-link
  data-content-href={href}
  title={href}
  onclick={(event) => handleRenderedLinkClick(event, destination.href)}
  onauxclick={(event) => {
    if (event.button === 1) handleRenderedLinkClick(event, destination.href);
  }}
>
  <span class="sr-only">{m('message_preview.open_linked_message')}: </span>
  {#if user}
    <span
      class="entity-link-image inline-flex size-[1.25em] shrink-0 self-center rounded-full [&>div]:size-full"
      aria-hidden="true"><UserAvatar {user} size="xs" useLiveProfile={false} /></span
    >
  {/if}
  <span class="iconify icon-[uil--comment-alt-message] shrink-0 self-center" aria-hidden="true"
  ></span>
  <bdi class="truncate">{user ? m('message_preview.message_from', { name }) : context}</bdi>
  {#if user && context}<span class="min-w-0 truncate text-muted">· <bdi>{context}</bdi></span>{/if}
</a>
