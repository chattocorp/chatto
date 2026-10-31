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
  class="entity-link"
  data-entity-kind={user ? 'user' : 'message'}
  data-message-link
  data-content-href={href}
  title={href}
  onclick={(event) => handleRenderedLinkClick(event, destination.href)}
>
  <span class="sr-only">{m('message_preview.open_linked_message')}: </span>
  <span
    class="link-pill-main"
    class:link-pill-main-image={!!user}
    class:link-pill-main-avatar={!!user}
  >
    <span class="link-pill-icon" class:link-pill-icon-image={!!user}>
      {#if user}
        <span
          class="entity-link-image m-px inline-flex size-[calc(100%-2px)] shrink-0 rounded-full [&>div]:size-full"
          aria-hidden="true"
          ><UserAvatar {user} size="xs" mutedPlaceholder useLiveProfile={false} /></span
        >
      {:else}
        <span
          class="iconify icon-[uil--comment-alt-message] shrink-0 self-center"
          aria-hidden="true"
        ></span>
      {/if}
    </span>
    <bdi class="min-w-0 truncate"
      >{user ? m('message_preview.message_from', { name }) : context}</bdi
    ></span
  >
  {#if user && context}<span class="link-pill-meta"><bdi class="truncate">{context}</bdi></span
    >{/if}
</a>
