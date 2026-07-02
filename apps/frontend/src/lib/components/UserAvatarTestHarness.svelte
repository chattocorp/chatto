<script lang="ts">
  import { PresenceStatus, type UserAvatarUserView } from '$lib/render/types';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

  type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

  let {
    size = 'md',
    showPresence = false,
    showStatus = false
  }: {
    size?: Size;
    showPresence?: boolean;
    showStatus?: boolean;
  } = $props();

  const user: UserAvatarUserView = {
    id: 'user-1',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online,
    customStatus: {
      emoji: '🍜',
      text: 'chatto:status:out_for_lunch',
      expiresAt: null
    }
  };

  createUserProfileCache();
  createPresenceCache();
</script>

<UserAvatar {user} {size} {showPresence} {showStatus} />
