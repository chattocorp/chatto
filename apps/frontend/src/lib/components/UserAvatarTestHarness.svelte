<script lang="ts">
  import { PresenceStatus, type UserAvatarUserView } from '$lib/render/types';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

  type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

  let {
    size = 'md',
    showPresence = false,
    showStatus = false,
    presenceStatus = PresenceStatus.Online
  }: {
    size?: Size;
    showPresence?: boolean;
    showStatus?: boolean;
    presenceStatus?: PresenceStatus;
  } = $props();

  const user = $derived({
    id: 'user-1',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    avatarUrl: null,
    presenceStatus,
    customStatus: {
      emoji: '🍜',
      text: 'chatto:status:out_for_lunch',
      expiresAt: null
    }
  } satisfies UserAvatarUserView);

  createUserProfileCache();
  createPresenceCache();
</script>

<UserAvatar {user} {size} {showPresence} {showStatus} />
