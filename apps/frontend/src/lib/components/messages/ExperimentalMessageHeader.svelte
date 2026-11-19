<!-- @component One-line header for the New message header experiment. The message row
owns author interactions and badges. This layout owns identity and timestamp presentation. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import MessageAuthorIdentity from './MessageAuthorIdentity.svelte';

  let {
    author,
    displayName,
    headerMeta,
    timelineClock
  }: {
    author: Snippet<[identity: Snippet<[UserAvatarUserView]>, fallbackClass: string]>;
    displayName: string;
    headerMeta?: Snippet;
    timelineClock: boolean;
  } = $props();
</script>

{#snippet identity(actor: UserAvatarUserView)}
  <MessageAuthorIdentity name={displayName} {actor} />
{/snippet}

<div class="flex min-w-0 items-baseline gap-x-2 whitespace-nowrap">
  {@render author(identity, 'min-w-0 truncate')}
  {#if headerMeta}
    <span
      class={[
        'contents [&>*]:[font-size:inherit]',
        timelineClock ? 'text-xs [&>*]:shrink-0 [&>*]:text-muted/60' : '[&>time]:truncate'
      ]}
    >
      <span aria-hidden="true" class="shrink-0 text-muted/60 only:hidden">·</span>
      {@render headerMeta()}
    </span>
  {/if}
</div>
