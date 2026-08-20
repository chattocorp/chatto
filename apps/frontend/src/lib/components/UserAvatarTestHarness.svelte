<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { UserAccountKind } from '@chatto/api-types/api/v1/users_pb';
  import type { UserAvatarUserView } from '$lib/render/users';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

  type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

  let {
    size = 'md',
    showPresence = false,
    showStatus = false,
    presenceStatus = PresenceStatus.ONLINE,
    accountKind = UserAccountKind.HUMAN
  }: {
    size?: Size;
    showPresence?: boolean;
    showStatus?: boolean;
    presenceStatus?: PresenceStatus;
    accountKind?: UserAccountKind;
  } = $props();

  const user = $derived({
    id: 'user-1',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    accountKind,
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

<UserAvatar {user} serverId="test-server" {size} {showPresence} {showStatus} />
