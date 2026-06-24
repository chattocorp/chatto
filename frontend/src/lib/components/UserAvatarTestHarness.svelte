<script lang="ts">
  import { PresenceStatus, type UserAvatarUserFragment } from '$lib/gql/graphql';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

  type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

  let {
    size = 'md'
  }: {
    size?: Size;
  } = $props();

  const user: UserAvatarUserFragment = {
    __typename: 'User',
    id: 'user-1',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online,
    customStatus: {
      __typename: 'CustomUserStatus',
      emoji: '🍜',
      text: 'chatto:status:out_for_lunch',
      expiresAt: null
    }
  };

  createUserProfileCache();
  createPresenceCache();
</script>

<UserAvatar {user} {size} />
