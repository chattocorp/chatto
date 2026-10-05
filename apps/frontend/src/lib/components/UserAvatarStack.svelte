<!-- @component A compact stack of one or more user avatars for activity rows. -->
<script lang="ts">
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import UserAvatar from './UserAvatar.svelte';

  let {
    users,
    useLiveProfile = true,
    testId,
    class: className = ''
  }: {
    users: readonly UserAvatarUserView[];
    useLiveProfile?: boolean;
    testId?: string;
    class?: string;
  } = $props();
</script>

<span class={['flex shrink-0', users.length > 1 && '-space-x-2', className]} data-testid={testId}>
  {#each users as user (user.id)}
    <UserAvatar
      {user}
      size="md"
      {useLiveProfile}
      class={users.length > 1 ? 'ring-2 ring-background' : ''}
    />
  {/each}
</span>
