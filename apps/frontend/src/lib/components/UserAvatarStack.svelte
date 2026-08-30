<!-- @component A compact stack of one or more user avatars for activity rows. -->
<script lang="ts">
  import type { UserAvatarUserView } from '$lib/render/users';
  import UserAvatar from './UserAvatar.svelte';

  let {
    users,
    serverId,
    useLiveProfile = true,
    testId,
    class: className = ''
  }: {
    users: readonly UserAvatarUserView[];
    serverId?: string;
    useLiveProfile?: boolean;
    testId?: string;
    class?: string;
  } = $props();
</script>

<span
  class={['flex shrink-0', users.length > 1 && '-space-x-2 rtl:space-x-reverse', className]}
  data-testid={testId}
>
  {#each users as user (user.id)}
    <UserAvatar
      {user}
      {serverId}
      size="md"
      {useLiveProfile}
      class={users.length > 1 ? 'ring-2 ring-background' : ''}
    />
  {/each}
</span>
