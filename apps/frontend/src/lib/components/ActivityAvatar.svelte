<!--
@component

Leading visual for an activity-list row: one fixed-size actor avatar, so row
text always lines up. Optional parts:

- `server`: a small server logo before the avatar, which the avatar overlaps
  slightly. Use it for app-wide lists that combine several servers. The logo
  is decorative: also provide the server name as text in the row, which can
  be visually hidden when the logo is sufficient.
- `badge`: content in a small circle at the bottom end corner, for example a
  cause icon, an emoji, or a second participant's avatar.
- `indicator`: content at the top end corner, for example an `UnreadDot`.
-->
<script module lang="ts">
  /** Server marker data for app-wide lists that combine several servers. */
  export type ActivityServer = { name: string; hostname: string; logoUrl: string | null };
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import ServerLogo from './ServerLogo.svelte';
  import UserAvatar from './UserAvatar.svelte';

  let {
    actor,
    size = 'md',
    server = null,
    badge,
    indicator
  }: {
    /** The actor to show; null shows a neutral placeholder. */
    actor: UserAvatarUserView | null;
    /** Small avatars suit compact rows; medium is the default activity-list size. */
    size?: 'sm' | 'md';
    server?: ActivityServer | null;
    badge?: Snippet;
    indicator?: Snippet;
  } = $props();
</script>

<span class="flex shrink-0 items-center">
  {#if server}
    <!-- Shrink the gutter logo here: ServerLogo is in the initial bundle. -->
    <span
      class={[
        'flex shrink-0 [&>div]:rounded-lg [&>div]:text-sm',
        size === 'sm' ? 'size-6' : 'size-7'
      ]}
      title={server.name === server.hostname
        ? server.hostname
        : `${server.name} (${server.hostname})`}
      aria-hidden="true"
      data-testid="activity-server"
    >
      <ServerLogo {server} fill />
    </span>
  {/if}
  <span
    class={['relative flex shrink-0', size === 'sm' ? 'size-8' : 'size-10', server && '-ms-1']}
    aria-hidden="true"
  >
    {#if actor}
      <UserAvatar user={actor} {size} class={server ? 'ring-2 ring-background' : ''} />
    {:else}
      <span
        class={[
          'flex size-full items-center justify-center rounded-full bg-surface-emphasized text-muted',
          server && 'ring-2 ring-background'
        ]}
      >
        <span class="iconify icon-[uil--user] text-lg" aria-hidden="true"></span>
      </span>
    {/if}
    {#if badge}
      <span
        class={[
          'absolute -end-1 -bottom-1 flex items-center justify-center overflow-hidden rounded-full bg-surface-emphasized text-xs text-muted ring-2 ring-background',
          size === 'sm' ? 'size-4' : 'size-5'
        ]}
        data-testid="activity-badge"
      >
        {@render badge()}
      </span>
    {/if}
    {#if indicator}
      <span class="absolute -end-1 -top-1 flex">{@render indicator()}</span>
    {/if}
  </span>
</span>
