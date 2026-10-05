<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

  type Size = 'xs' | 'sm' | 'md' | 'message' | 'lg' | 'xl';

  let {
    size = 'md',
    showPresence = false,
    showStatus = false,
    presenceStatus = PresenceStatus.ONLINE,
    presence,
    isBot = false
  }: {
    size?: Size;
    showPresence?: boolean;
    showStatus?: boolean;
    presenceStatus?: PresenceStatus;
    /** Current presence from the server store, which overrides `presenceStatus`. */
    presence?: PresenceStatus;
    isBot?: boolean;
  } = $props();

  const user = $derived({
    id: 'user-1',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    isBot,
    avatarUrl: null,
    presenceStatus,
    customStatus: {
      emoji: '🍜',
      text: 'chatto:status:out_for_lunch',
      expiresAt: null
    }
  } satisfies UserAvatarUserView);

  provideUserProfiles();
</script>

<UserAvatar {user} {presence} {size} {showPresence} {showStatus} />
